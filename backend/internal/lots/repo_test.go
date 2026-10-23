package lots_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"lot/internal/closer"
	"lot/internal/db"
	"lot/internal/lots"
)

// Тесты работают с реальным PostgreSQL:
//
//	createdb lot_test
//	TEST_DATABASE_URL=postgres://localhost:5432/lot_test?sslmode=disable go test -race ./...
func setup(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL не задан")
	}
	ctx := context.Background()
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := db.Migrate(ctx, pool, slog.Default()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE bids, lot_images, lots, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	return pool
}

func createUsers(t *testing.T, pool *pgxpool.Pool, n int) []int64 {
	t.Helper()
	ids := make([]int64, n)
	for i := range ids {
		err := pool.QueryRow(context.Background(),
			`INSERT INTO users (email, password_hash, display_name) VALUES ($1, 'x', $2) RETURNING id`,
			fmt.Sprintf("u%d@example.test", i), fmt.Sprintf("User %d", i),
		).Scan(&ids[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	return ids
}

func createLot(t *testing.T, repo *lots.Repo, pool *pgxpool.Pool, seller int64, ends time.Duration) int64 {
	t.Helper()
	var catID int64
	pool.QueryRow(context.Background(), `SELECT id FROM categories LIMIT 1`).Scan(&catID)
	id, err := repo.Create(context.Background(), lots.CreateLot{
		SellerID: seller, CategoryID: catID, Title: "Test lot",
		StartPrice: 1000, MinStep: 100, EndsAt: time.Now().Add(ends),
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// 100 участников одновременно ставят одну и ту же сумму — принята должна быть ровно одна ставка.
func TestConcurrentSameAmount(t *testing.T) {
	pool := setup(t)
	repo := lots.NewRepo(pool, 0)
	users := createUsers(t, pool, 101)
	lotID := createLot(t, repo, pool, users[0], time.Hour)

	var accepted, tooLow atomic.Int32
	var wg sync.WaitGroup
	for _, u := range users[1:] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.PlaceBid(context.Background(), lotID, u, 1000)
			var low *lots.BidTooLowError
			switch {
			case err == nil:
				accepted.Add(1)
			case errors.As(err, &low):
				tooLow.Add(1)
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	if accepted.Load() != 1 || tooLow.Load() != 99 {
		t.Fatalf("accepted=%d tooLow=%d, want 1/99", accepted.Load(), tooLow.Load())
	}
	assertInvariants(t, pool, lotID)
}

// Много участников повышают ставки параллельно — история ставок должна быть строго
// возрастающей с учётом шага, а состояние лота согласовано с таблицей bids.
func TestConcurrentRisingBids(t *testing.T) {
	pool := setup(t)
	repo := lots.NewRepo(pool, 0)
	users := createUsers(t, pool, 21)
	lotID := createLot(t, repo, pool, users[0], time.Hour)

	var wg sync.WaitGroup
	for _, u := range users[1:] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 30 {
				lot, err := repo.Get(context.Background(), lotID)
				if err != nil {
					t.Error(err)
					return
				}
				repo.PlaceBid(context.Background(), lotID, u, lot.MinNextBid)
			}
		}()
	}
	wg.Wait()
	assertInvariants(t, pool, lotID)
}

// Ставки идут непрерывно, пока работает автозакрытие: после закрытия не должно
// появиться ни одной ставки, а победитель — автор максимальной ставки.
func TestCloseWhileBidding(t *testing.T) {
	pool := setup(t)
	repo := lots.NewRepo(pool, 0)
	users := createUsers(t, pool, 11)
	lotID := createLot(t, repo, pool, users[0], 1500*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	closedCh := make(chan closer.Closed, 1)
	go closer.Run(ctx, pool, 50*time.Millisecond, slog.Default(), func(c closer.Closed) { closedCh <- c })

	var closedErrs atomic.Int32
	var wg sync.WaitGroup
	for _, u := range users[1:] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				lot, err := repo.Get(ctx, lotID)
				if err != nil {
					return
				}
				_, err = repo.PlaceBid(ctx, lotID, u, lot.MinNextBid)
				if errors.Is(err, lots.ErrAuctionClosed) {
					closedErrs.Add(1)
					return
				}
			}
		}()
	}

	var c closer.Closed
	select {
	case c = <-closedCh:
	case <-time.After(10 * time.Second):
		t.Fatal("лот не закрылся")
	}
	wg.Wait()

	if closedErrs.Load() != int32(len(users)-1) {
		t.Errorf("auction_closed получили %d участников из %d", closedErrs.Load(), len(users)-1)
	}

	var maxAmount, maxUser int64
	var lastBidAt time.Time
	pool.QueryRow(ctx, `SELECT amount, user_id, created_at FROM bids WHERE lot_id = $1 ORDER BY amount DESC LIMIT 1`, lotID).
		Scan(&maxAmount, &maxUser, &lastBidAt)
	if c.FinalPrice == nil || *c.FinalPrice != maxAmount || *c.WinnerID != maxUser {
		t.Errorf("победитель %v/%v, ожидался %d/%d", c.WinnerID, c.FinalPrice, maxUser, maxAmount)
	}
	if lastBidAt.After(c.ClosedAt) {
		t.Errorf("ставка %v позже закрытия %v", lastBidAt, c.ClosedAt)
	}
	assertInvariants(t, pool, lotID)
}

func assertInvariants(t *testing.T, pool *pgxpool.Pool, lotID int64) {
	t.Helper()
	ctx := context.Background()

	var startPrice, minStep int64
	var current, leader *int64
	var bidCount int
	pool.QueryRow(ctx, `SELECT start_price, min_step, current_price, leader_id, bid_count FROM lots WHERE id = $1`, lotID).
		Scan(&startPrice, &minStep, &current, &leader, &bidCount)

	rows, _ := pool.Query(ctx, `SELECT user_id, amount FROM bids WHERE lot_id = $1 ORDER BY id`, lotID)
	defer rows.Close()
	var n int
	var prevAmount, lastUser int64
	for rows.Next() {
		var user, amount int64
		rows.Scan(&user, &amount)
		if n == 0 && amount < startPrice {
			t.Errorf("первая ставка %d ниже стартовой цены %d", amount, startPrice)
		}
		if n > 0 && amount < prevAmount+minStep {
			t.Errorf("ставка #%d = %d, предыдущая %d, шаг %d", n, amount, prevAmount, minStep)
		}
		prevAmount, lastUser = amount, user
		n++
	}
	if n != bidCount {
		t.Errorf("bid_count=%d, ставок в таблице %d", bidCount, n)
	}
	if n > 0 && (current == nil || *current != prevAmount || *leader != lastUser) {
		t.Errorf("current_price/leader не совпадают с последней ставкой")
	}
	t.Logf("ставок принято: %d, итоговая цена: %d", n, prevAmount)
}

// Снятие лота во время параллельных ставок: после отмены не принимается ни одна ставка,
// повторно снять лот (или снять завершённый) нельзя.
func TestCancelWhileBidding(t *testing.T) {
	pool := setup(t)
	repo := lots.NewRepo(pool, 0)
	users := createUsers(t, pool, 11)
	lotID := createLot(t, repo, pool, users[0], time.Hour)
	ctx := context.Background()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for _, u := range users[1:] {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				lot, err := repo.Get(ctx, lotID)
				if err != nil {
					return
				}
				if _, err := repo.PlaceBid(ctx, lotID, u, lot.MinNextBid); errors.Is(err, lots.ErrLotCancelled) {
					return
				}
			}
		}()
	}

	time.Sleep(200 * time.Millisecond)
	c, err := repo.Cancel(ctx, lotID, "Подделка")
	if err != nil {
		t.Fatal(err)
	}
	close(stop)
	wg.Wait()

	var lastBidAt time.Time
	pool.QueryRow(ctx, `SELECT COALESCE(max(created_at), 'epoch') FROM bids WHERE lot_id = $1`, lotID).Scan(&lastBidAt)
	if lastBidAt.After(c.ClosedAt) {
		t.Errorf("ставка %v принята после снятия лота %v", lastBidAt, c.ClosedAt)
	}
	if _, err := repo.PlaceBid(ctx, lotID, users[1], 1_000_000); !errors.Is(err, lots.ErrLotCancelled) {
		t.Errorf("ставка на снятый лот: %v, ожидалась ErrLotCancelled", err)
	}
	if _, err := repo.Cancel(ctx, lotID, "ещё раз"); !errors.Is(err, lots.ErrAuctionClosed) {
		t.Errorf("повторное снятие: %v, ожидалась ErrAuctionClosed", err)
	}
	if _, err := repo.Cancel(ctx, 999999, "нет лота"); !errors.Is(err, lots.ErrLotNotFound) {
		t.Errorf("несуществующий лот: %v", err)
	}
	assertInvariants(t, pool, lotID)
}
