package lots

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repo struct {
	pool        *pgxpool.Pool
	antiSniping time.Duration
}

func NewRepo(pool *pgxpool.Pool, antiSniping time.Duration) *Repo {
	return &Repo{pool: pool, antiSniping: antiSniping}
}

const lotSelect = `
SELECT l.id, l.seller_id, s.display_name, l.category_id, c.slug, c.name,
       l.title, l.description, l.start_price, l.min_step,
       l.current_price, l.leader_id, ld.display_name, l.bid_count,
       l.starts_at, l.ends_at, l.status,
       l.winner_id, w.display_name, l.final_price, l.closed_at, l.cancel_reason,
       (SELECT i.url FROM lot_images i WHERE i.lot_id = l.id ORDER BY i.position, i.id LIMIT 1)
FROM lots l
JOIN users s       ON s.id = l.seller_id
JOIN categories c  ON c.id = l.category_id
LEFT JOIN users ld ON ld.id = l.leader_id
LEFT JOIN users w  ON w.id = l.winner_id`

func scanLot(row pgx.Row) (Lot, error) {
	var l Lot
	err := row.Scan(&l.ID, &l.SellerID, &l.SellerName, &l.CategoryID, &l.CategorySlug, &l.CategoryName,
		&l.Title, &l.Description, &l.StartPrice, &l.MinStep,
		&l.CurrentPrice, &l.LeaderID, &l.LeaderName, &l.BidCount,
		&l.StartsAt, &l.EndsAt, &l.Status,
		&l.WinnerID, &l.WinnerName, &l.FinalPrice, &l.ClosedAt, &l.CancelReason, &l.CoverURL)
	l.MinNextBid = MinNextBid(l.StartPrice, l.MinStep, l.CurrentPrice)
	return l, err
}

func (r *Repo) Categories(ctx context.Context) ([]Category, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, name, slug FROM categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Category])
}

type ListFilter struct {
	Category string // slug
	Status   string // active | finished | cancelled; пусто — все (для админ-панели)
	Query    string
	Sort     string // ending | newest | price_asc | price_desc | popular
	Limit    int
	Offset   int
}

func (r *Repo) List(ctx context.Context, f ListFilter) ([]Lot, int, error) {
	var (
		where []string
		args  []any
	)
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	where = append(where, "TRUE")
	if f.Status != "" {
		where = append(where, "l.status = "+arg(f.Status))
	}
	if f.Category != "" {
		where = append(where, "c.slug = "+arg(f.Category))
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, "l.title ILIKE "+arg("%"+escapeLike(q)+"%"))
	}
	whereSQL := " WHERE " + strings.Join(where, " AND ")

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM lots l JOIN categories c ON c.id = l.category_id`+whereSQL, args...,
	).Scan(&total); err != nil {
		return nil, 0, err
	}

	order := map[string]string{
		"ending":     "l.ends_at ASC",
		"newest":     "l.created_at DESC",
		"price_asc":  "COALESCE(l.current_price, l.start_price) ASC",
		"price_desc": "COALESCE(l.current_price, l.start_price) DESC",
		"popular":    "l.bid_count DESC",
	}[f.Sort]
	if order == "" || (f.Sort == "ending" && f.Status != "active") {
		order = "l.ends_at DESC"
	}

	sql := lotSelect + whereSQL + " ORDER BY " + order + ", l.id DESC LIMIT " + arg(f.Limit) + " OFFSET " + arg(f.Offset)
	items, err := r.queryLots(ctx, sql, args...)
	return items, total, err
}

func (r *Repo) queryLots(ctx context.Context, sql string, args ...any) ([]Lot, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Lot{}
	for rows.Next() {
		l, err := scanLot(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, l)
	}
	return items, rows.Err()
}

// UserBids — лоты, в торгах по которым участвовал пользователь, с его максимальной ставкой.
// Сначала активные (ближайшие к завершению), затем недавно завершённые.
func (r *Repo) UserBids(ctx context.Context, userID int64) ([]Participation, error) {
	lots, err := r.queryLots(ctx, lotSelect+`
		WHERE l.id IN (SELECT lot_id FROM bids WHERE user_id = $1)
		ORDER BY (l.status = 'active') DESC,
		         CASE WHEN l.status = 'active' THEN l.ends_at END ASC,
		         l.closed_at DESC
		LIMIT 200`, userID)
	if err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT lot_id, max(amount) FROM bids WHERE user_id = $1 GROUP BY lot_id`, userID)
	if err != nil {
		return nil, err
	}
	myMax := map[int64]int64{}
	var lotID, amount int64
	if _, err := pgx.ForEachRow(rows, []any{&lotID, &amount}, func() error {
		myMax[lotID] = amount
		return nil
	}); err != nil {
		return nil, err
	}

	res := make([]Participation, len(lots))
	for i, l := range lots {
		res[i] = Participation{Lot: l, MyMaxBid: myMax[l.ID]}
	}
	return res, nil
}

// UserLots — лоты, выставленные пользователем.
func (r *Repo) UserLots(ctx context.Context, userID int64) ([]Lot, error) {
	return r.queryLots(ctx, lotSelect+`
		WHERE l.seller_id = $1
		ORDER BY (l.status = 'active') DESC, l.created_at DESC
		LIMIT 200`, userID)
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (r *Repo) Get(ctx context.Context, id int64) (Lot, error) {
	l, err := scanLot(r.pool.QueryRow(ctx, lotSelect+" WHERE l.id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Lot{}, ErrLotNotFound
	}
	if err != nil {
		return Lot{}, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, url, position FROM lot_images WHERE lot_id = $1 ORDER BY position, id`, id)
	if err != nil {
		return Lot{}, err
	}
	l.Images, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Image])
	return l, err
}

func (r *Repo) Bids(ctx context.Context, lotID int64, limit int) ([]Bid, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT b.id, b.lot_id, b.user_id, u.display_name, b.amount, b.created_at
		FROM bids b JOIN users u ON u.id = b.user_id
		WHERE b.lot_id = $1
		ORDER BY b.id DESC
		LIMIT $2`, lotID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Bid])
}

type CreateLot struct {
	SellerID    int64
	CategoryID  int64
	Title       string
	Description string
	StartPrice  int64
	MinStep     int64
	EndsAt      time.Time
	Images      []string
}

func (r *Repo) Create(ctx context.Context, c CreateLot) (int64, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO lots (seller_id, category_id, title, description, start_price, min_step, ends_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		c.SellerID, c.CategoryID, c.Title, c.Description, c.StartPrice, c.MinStep, c.EndsAt,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	for i, url := range c.Images {
		if _, err := tx.Exec(ctx,
			`INSERT INTO lot_images (lot_id, url, position) VALUES ($1, $2, $3)`, id, url, i,
		); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit(ctx)
}

// PlaceBid принимает ставку атомарно.
//
// Строка лота блокируется (SELECT ... FOR UPDATE), поэтому конкурентные ставки
// на один лот выполняются строго по очереди: каждая следующая видит цену,
// уже обновлённую предыдущей, и проверяется относительно неё. Закрытие торгов
// (closer) пропускает заблокированные строки (SKIP LOCKED), а ставка после
// закрытия увидит status = 'finished' — гонки «ставка в последнюю миллисекунду» нет.
func (r *Repo) PlaceBid(ctx context.Context, lotID, userID, amount int64) (BidPlaced, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return BidPlaced{}, err
	}
	defer tx.Rollback(ctx)

	var (
		sellerID, startPrice, minStep int64
		current, leader               *int64
		status, title                 string
		startsAt, endsAt              time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT seller_id, status, title, start_price, min_step, current_price, leader_id, starts_at, ends_at
		FROM lots WHERE id = $1
		FOR UPDATE`, lotID,
	).Scan(&sellerID, &status, &title, &startPrice, &minStep, &current, &leader, &startsAt, &endsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return BidPlaced{}, ErrLotNotFound
	}
	if err != nil {
		return BidPlaced{}, err
	}

	// Время берём из БД уже после получения блокировки: это единые часы
	// и для ставок, и для фонового закрытия торгов.
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return BidPlaced{}, err
	}

	switch {
	case status == "cancelled":
		return BidPlaced{}, ErrLotCancelled
	case status != "active" || !now.Before(endsAt):
		return BidPlaced{}, ErrAuctionClosed
	case now.Before(startsAt):
		return BidPlaced{}, ErrNotStarted
	case sellerID == userID:
		return BidPlaced{}, ErrOwnLot
	case leader != nil && *leader == userID:
		return BidPlaced{}, ErrAlreadyLeading
	}

	if min := MinNextBid(startPrice, minStep, current); amount < min {
		return BidPlaced{}, &BidTooLowError{MinNextBid: min}
	}

	newEndsAt, extended := endsAt, false
	if r.antiSniping > 0 && endsAt.Sub(now) < r.antiSniping {
		newEndsAt, extended = now.Add(r.antiSniping), true
	}

	res := BidPlaced{
		LotID:        lotID,
		CurrentPrice: amount,
		LeaderID:     userID,
		MinNextBid:   amount + minStep,
		EndsAt:       newEndsAt,
		Extended:     extended,
		LotTitle:     title,
		PrevLeaderID: leader,
	}
	if err := tx.QueryRow(ctx, `
		UPDATE lots SET current_price = $2, leader_id = $3, bid_count = bid_count + 1, ends_at = $4
		WHERE id = $1
		RETURNING bid_count`, lotID, amount, userID, newEndsAt,
	).Scan(&res.BidCount); err != nil {
		return BidPlaced{}, err
	}

	res.Bid = Bid{LotID: lotID, UserID: userID, Amount: amount, CreatedAt: now}
	if err := tx.QueryRow(ctx, `
		WITH ins AS (
			INSERT INTO bids (lot_id, user_id, amount, created_at) VALUES ($1, $2, $3, $4) RETURNING id
		)
		SELECT ins.id, u.display_name FROM ins, users u WHERE u.id = $2`,
		lotID, userID, amount, now,
	).Scan(&res.Bid.ID, &res.Bid.UserName); err != nil {
		return BidPlaced{}, err
	}
	res.LeaderName = res.Bid.UserName

	return res, tx.Commit(ctx)
}

// Cancel снимает активный лот с торгов. UPDATE берёт ту же блокировку строки,
// что и PlaceBid, поэтому после отмены ни одна ставка уже не пройдёт.
// closed_at = clock_timestamp(), а не now(): now() — время начала запроса, до
// ожидания блокировки, и оно оказалось бы раньше ставки, которая держала строку.
func (r *Repo) Cancel(ctx context.Context, lotID int64, reason string) (Cancelled, error) {
	c := Cancelled{LotID: lotID, Reason: reason}
	err := r.pool.QueryRow(ctx, `
		UPDATE lots SET status = 'cancelled', cancel_reason = $2, closed_at = clock_timestamp()
		WHERE id = $1 AND status = 'active'
		RETURNING title, seller_id, leader_id, closed_at`, lotID, reason,
	).Scan(&c.LotTitle, &c.SellerID, &c.LeaderID, &c.ClosedAt)
	if !errors.Is(err, pgx.ErrNoRows) {
		return c, err
	}
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM lots WHERE id = $1)`, lotID).Scan(&exists); err != nil {
		return c, err
	}
	if !exists {
		return c, ErrLotNotFound
	}
	return c, ErrAuctionClosed
}

// CancelBySeller снимает все активные лоты продавца (при блокировке аккаунта).
func (r *Repo) CancelBySeller(ctx context.Context, sellerID int64, reason string) ([]Cancelled, error) {
	rows, err := r.pool.Query(ctx, `
		UPDATE lots SET status = 'cancelled', cancel_reason = $2, closed_at = clock_timestamp()
		WHERE seller_id = $1 AND status = 'active'
		RETURNING id, title, seller_id, $2::text, closed_at, leader_id`, sellerID, reason)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Cancelled])
}
