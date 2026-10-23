// Package closer автоматически завершает торги по истечении таймера и фиксирует победителя.
package closer

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Closed struct {
	LotID      int64     `json:"lotId"`
	LotTitle   string    `json:"lotTitle"`
	SellerID   int64     `json:"sellerId"`
	WinnerID   *int64    `json:"winnerId"`
	WinnerName *string   `json:"winnerName"`
	FinalPrice *int64    `json:"finalPrice"`
	ClosedAt   time.Time `json:"closedAt"`
}

// Один атомарный запрос: выбрать истёкшие лоты, перевести в finished и
// записать победителя (= лидер на момент закрытия; NULL, если ставок не было).
//
// FOR UPDATE SKIP LOCKED пропускает лоты, на которые прямо сейчас идёт ставка:
// ставка может продлить таймер, поэтому такой лот проверится на следующем тике.
// Это же позволяет безопасно запускать несколько экземпляров сервера.
const closeDueSQL = `
WITH due AS (
    SELECT id FROM lots
    WHERE status = 'active' AND ends_at <= now()
    ORDER BY ends_at
    LIMIT 500
    FOR UPDATE SKIP LOCKED
), closed AS (
    UPDATE lots l
    SET status = 'finished', winner_id = l.leader_id, final_price = l.current_price, closed_at = now()
    FROM due
    WHERE l.id = due.id
    RETURNING l.id, l.title, l.seller_id, l.winner_id, l.final_price, l.closed_at
)
SELECT c.id, c.title, c.seller_id, c.winner_id, u.display_name, c.final_price, c.closed_at
FROM closed c LEFT JOIN users u ON u.id = c.winner_id`

func CloseDue(ctx context.Context, pool *pgxpool.Pool) ([]Closed, error) {
	rows, err := pool.Query(ctx, closeDueSQL)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Closed])
}

// Run проверяет истёкшие лоты каждые interval до отмены ctx.
func Run(ctx context.Context, pool *pgxpool.Pool, interval time.Duration, log *slog.Logger, onClosed func(Closed)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		closed, err := CloseDue(ctx, pool)
		if err != nil {
			if ctx.Err() == nil {
				log.Error("close due lots", "err", err)
			}
			continue
		}
		for _, c := range closed {
			if c.WinnerID != nil {
				log.Info("auction closed", "lot", c.LotID, "winner", *c.WinnerID, "price", *c.FinalPrice)
			} else {
				log.Info("auction closed without bids", "lot", c.LotID)
			}
			onClosed(c)
		}
	}
}
