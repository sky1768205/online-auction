// Нагрузочный тест: много WebSocket-наблюдателей на одном лоте и параллельные ставки.
//
//	go run ./cmd/loadtest -watchers 1000 -bidders 50 -duration 20s
//
// Что измеряется:
//   - задержка доставки bid:new всем наблюдателям (от отправки POST до получения события);
//   - полнота доставки (каждый наблюдатель должен получить каждую принятую ставку);
//   - время ответа POST /bids и распределение отказов по причинам;
//   - доставка lot:closed после автозакрытия;
//   - инварианты в БД: ставки строго растут на шаг, bid_count и победитель согласованы.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"lot/internal/config"
	"lot/internal/db"
)

var (
	base      = flag.String("base", "http://localhost:8080", "адрес API")
	watchers  = flag.Int("watchers", 500, "число WebSocket-наблюдателей")
	bidders   = flag.Int("bidders", 50, "число одновременно торгующихся участников")
	duration  = flag.Duration("duration", 20*time.Second, "длительность фазы ставок")
	think     = flag.Duration("think", 0, "пауза участника между ставками")
	checkDB   = flag.Bool("check-db", true, "проверить инварианты напрямую в БД (DATABASE_URL)")
	transport = &http.Transport{MaxIdleConnsPerHost: 1024, MaxConnsPerHost: 0}
)

type event struct {
	Type string          `json:"type"`
	Room string          `json:"room"`
	Data json.RawMessage `json:"data"`
}

type bidPlaced struct {
	LotID        int64 `json:"lotId"`
	CurrentPrice int64 `json:"currentPrice"`
	MinNextBid   int64 `json:"minNextBid"`
}

type stats struct {
	mu        sync.Mutex
	broadcast []time.Duration
	post      []time.Duration
	codes     map[string]int
}

func main() {
	flag.Parse()
	ctx := context.Background()
	runID := time.Now().UnixNano()
	st := &stats{codes: map[string]int{}}

	// 1. Пользователи: продавец + участники торгов.
	fmt.Printf("Регистрация %d участников...\n", *bidders+1)
	clients := make([]*http.Client, *bidders+1)
	parallel(len(clients), 16, func(i int) {
		jar, _ := cookiejar.New(nil)
		clients[i] = &http.Client{Jar: jar, Transport: transport, Timeout: 30 * time.Second}
		body := map[string]string{
			"email":       fmt.Sprintf("loadtest-%d-%d@example.test", runID, i),
			"password":    fmt.Sprintf("lt-%d-%d", runID, rand.Int64()),
			"displayName": fmt.Sprintf("Бот %d", i),
		}
		if code, _ := post(clients[i], "/api/auth/register", body, nil); code != http.StatusCreated {
			log.Fatalf("регистрация пользователя %d: HTTP %d", i, code)
		}
	})

	// 2. Лот, торги по которому закончатся вскоре после фазы ставок.
	var cats []struct{ ID int64 }
	get("/api/categories", &cats)
	var lot struct {
		ID         int64
		MinStep    int64
		MinNextBid int64
	}
	code, _ := post(clients[0], "/api/lots", map[string]any{
		"title":      fmt.Sprintf("Нагрузочный тест %d", runID),
		"categoryId": cats[0].ID,
		"startPrice": 100_00,
		"minStep":    10_00,
		"endsAt":     time.Now().Add(*duration + 15*time.Second),
	}, &lot)
	if code != http.StatusCreated {
		log.Fatalf("создание лота: HTTP %d", code)
	}
	fmt.Printf("Лот #%d создан\n", lot.ID)

	var minNext atomic.Int64
	minNext.Store(lot.MinNextBid)
	var sentAt sync.Map // сумма ставки → время отправки POST

	// 3. Наблюдатели.
	fmt.Printf("Подключение %d WebSocket-наблюдателей...\n", *watchers)
	wsURL := strings.Replace(*base, "http", "ws", 1) + "/ws"
	room := fmt.Sprintf("lot:%d", lot.ID)
	received := make([]atomic.Int64, *watchers)
	var closedSeen atomic.Int64
	closedAll := make(chan struct{})
	var closedOnce sync.Once
	conns := make([]*websocket.Conn, *watchers)
	connectStart := time.Now()
	parallel(*watchers, 100, func(i int) {
		c, _, err := websocket.Dial(ctx, wsURL, nil)
		if err != nil {
			log.Fatalf("watcher %d: %v", i, err)
		}
		c.SetReadLimit(1 << 20)
		msg, _ := json.Marshal(map[string]string{"type": "subscribe", "room": room})
		c.Write(ctx, websocket.MessageText, msg)
		for { // ждём подтверждения подписки
			var e event
			if readEvent(ctx, c, &e) != nil {
				log.Fatalf("watcher %d: соединение закрыто до подписки", i)
			}
			if e.Type == "subscribed" {
				break
			}
		}
		conns[i] = c
	})
	fmt.Printf("Подключены за %v\n", time.Since(connectStart).Round(time.Millisecond))

	for i, c := range conns {
		go func() {
			var local []time.Duration
			defer func() {
				st.mu.Lock()
				st.broadcast = append(st.broadcast, local...)
				st.mu.Unlock()
			}()
			for {
				var e event
				if readEvent(ctx, c, &e) != nil {
					return
				}
				switch e.Type {
				case "bid:new":
					now := time.Now()
					var b bidPlaced
					json.Unmarshal(e.Data, &b)
					received[i].Add(1)
					if t, ok := sentAt.Load(b.CurrentPrice); ok {
						local = append(local, now.Sub(t.(time.Time)))
					}
					for { // поддерживаем актуальную минимальную ставку для участников
						cur := minNext.Load()
						if b.MinNextBid <= cur || minNext.CompareAndSwap(cur, b.MinNextBid) {
							break
						}
					}
				case "lot:closed":
					if closedSeen.Add(1) == int64(*watchers) {
						closedOnce.Do(func() { close(closedAll) })
					}
					return
				}
			}
		}()
	}

	// 4. Фаза ставок.
	fmt.Printf("Ставки: %d участников, %v...\n", *bidders, *duration)
	deadline := time.Now().Add(*duration)
	var accepted atomic.Int64
	var wg sync.WaitGroup
	for i := 1; i <= *bidders; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				amount := minNext.Load() + lot.MinStep*rand.Int64N(3)
				start := time.Now()
				sentAt.LoadOrStore(amount, start)
				var errBody struct{ Error string }
				code, _ := post(clients[i], fmt.Sprintf("/api/lots/%d/bids", lot.ID), map[string]int64{"amount": amount}, &errBody)
				elapsed := time.Since(start)

				key := errBody.Error
				if code == http.StatusCreated {
					key = "accepted"
					accepted.Add(1)
				} else if key == "" {
					key = fmt.Sprintf("http_%d", code)
				}
				st.mu.Lock()
				st.post = append(st.post, elapsed)
				st.codes[key]++
				st.mu.Unlock()

				if *think > 0 {
					time.Sleep(*think)
				}
			}
		}()
	}
	wg.Wait()
	bidPhase := *duration

	// 5. Ждём автозакрытия (с учётом продления «антиснайпинга»).
	fmt.Println("Ожидание автоматического закрытия торгов...")
	closeWaitStart := time.Now()
	select {
	case <-closedAll:
	case <-time.After(2 * time.Minute):
		fmt.Println("ВНИМАНИЕ: не все наблюдатели получили lot:closed за 2 минуты")
	}
	closeWait := time.Since(closeWaitStart)
	time.Sleep(200 * time.Millisecond)

	// 6. Отчёт.
	st.mu.Lock()
	defer st.mu.Unlock()
	acc := accepted.Load()
	var delivered, minRecv int64 = 0, acc
	for i := range received {
		n := received[i].Load()
		delivered += n
		minRecv = min(minRecv, n)
	}

	fmt.Println("\n================ РЕЗУЛЬТАТЫ ================")
	fmt.Printf("Наблюдателей (WebSocket): %d, участников: %d\n", *watchers, *bidders)
	fmt.Printf("Запросов на ставку: %d (%.0f/с), принято: %d (%.1f/с)\n",
		len(st.post), float64(len(st.post))/bidPhase.Seconds(), acc, float64(acc)/bidPhase.Seconds())
	fmt.Println("Ответы:")
	for _, k := range sortedKeys(st.codes) {
		fmt.Printf("  %-18s %d\n", k, st.codes[k])
	}
	fmt.Printf("POST /bids:            %s\n", percentiles(st.post))
	fmt.Printf("Доставка bid:new:      %s\n", percentiles(st.broadcast))
	fmt.Printf("Сообщений доставлено:  %d из %d ожидаемых (%.2f%%), минимум у одного клиента: %d/%d\n",
		delivered, acc*int64(*watchers), 100*float64(delivered)/float64(max(acc*int64(*watchers), 1)), minRecv, acc)
	fmt.Printf("lot:closed получили:   %d/%d (через %v после конца ставок)\n",
		closedSeen.Load(), *watchers, closeWait.Round(time.Millisecond))

	ok := minRecv == acc && closedSeen.Load() == int64(*watchers)
	if *checkDB {
		ok = checkInvariants(ctx, lot.ID, acc) && ok
	}
	if !ok {
		fmt.Println("\nИТОГ: ЕСТЬ НАРУШЕНИЯ")
		os.Exit(1)
	}
	fmt.Println("\nИТОГ: OK")
}

func checkInvariants(ctx context.Context, lotID, accepted int64) bool {
	pool, err := db.Connect(ctx, config.Load().DatabaseURL)
	if err != nil {
		fmt.Println("Проверка БД пропущена:", err)
		return true
	}
	defer pool.Close()

	var (
		status                    string
		bidCount                  int64
		minStep                   int64
		finalPrice, winner, maxID *int64
		violations, total         int64
		maxAmount, lastUser       *int64
	)
	pool.QueryRow(ctx, `SELECT status, bid_count, min_step, final_price, winner_id FROM lots WHERE id = $1`, lotID).
		Scan(&status, &bidCount, &minStep, &finalPrice, &winner)
	pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE amount < prev + $2), max(amount), max(id)
		FROM (SELECT id, amount, lag(amount) OVER (ORDER BY id) AS prev FROM bids WHERE lot_id = $1) t`,
		lotID, minStep).Scan(&total, &violations, &maxAmount, &maxID)
	if maxID != nil {
		pool.QueryRow(ctx, `SELECT user_id FROM bids WHERE id = $1`, *maxID).Scan(&lastUser)
	}

	fmt.Println("\nИнварианты в БД:")
	ok := true
	check := func(name string, cond bool) {
		mark := "OK"
		if !cond {
			mark, ok = "FAIL", false
		}
		fmt.Printf("  [%s] %s\n", mark, name)
	}
	check("лот в статусе finished", status == "finished")
	check(fmt.Sprintf("ставок в БД (%d) = принятых API (%d) = bid_count (%d)", total, accepted, bidCount),
		total == accepted && total == bidCount)
	check("каждая ставка ≥ предыдущей + шаг", violations == 0)
	check("final_price = максимальная ставка", eq(finalPrice, maxAmount))
	check("победитель = автор последней (максимальной) ставки", eq(winner, lastUser))
	return ok
}

func eq(a, b *int64) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func readEvent(ctx context.Context, c *websocket.Conn, e *event) error {
	_, data, err := c.Read(ctx)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, e)
}

func post(c *http.Client, path string, body, out any) (int, error) {
	b, _ := json.Marshal(body)
	resp, err := c.Post(*base+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out != nil {
		json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode, nil
}

func get(path string, out any) {
	u, _ := url.Parse(*base + path)
	resp, err := (&http.Client{Transport: transport}).Get(u.String())
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	json.NewDecoder(resp.Body).Decode(out)
}

func parallel(n, limit int, fn func(i int)) {
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			fn(i)
		}()
	}
	wg.Wait()
}

func percentiles(d []time.Duration) string {
	if len(d) == 0 {
		return "нет данных"
	}
	s := slices.Clone(d)
	slices.Sort(s)
	p := func(q float64) time.Duration { return s[int(q*float64(len(s)-1))].Round(10 * time.Microsecond) }
	return fmt.Sprintf("p50=%v p95=%v p99=%v max=%v (n=%d)", p(0.5), p(0.95), p(0.99), s[len(s)-1].Round(10*time.Microsecond), len(s))
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
