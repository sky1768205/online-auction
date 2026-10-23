// Package admin — API админ-панели: статистика, модерация лотов и пользователей.
// Все маршруты закрыты auth.RequireUser + auth.RequireAdmin.
package admin

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"lot/internal/auth"
	"lot/internal/httpx"
	"lot/internal/lots"
)

type Handlers struct {
	pool   *pgxpool.Pool
	lots   *lots.Repo
	hub    lots.Broadcaster
	online func() int64 // число WebSocket-подключений
	log    *slog.Logger
}

func NewHandlers(pool *pgxpool.Pool, repo *lots.Repo, hub lots.Broadcaster, online func() int64, log *slog.Logger) *Handlers {
	return &Handlers{pool: pool, lots: repo, hub: hub, online: online, log: log}
}

type Stats struct {
	Users         int   `json:"users"`
	BlockedUsers  int   `json:"blockedUsers"`
	ActiveLots    int   `json:"activeLots"`
	FinishedLots  int   `json:"finishedLots"`
	SoldLots      int   `json:"soldLots"`
	CancelledLots int   `json:"cancelledLots"`
	Bids          int   `json:"bids"`
	Bids24h       int   `json:"bids24h"`
	Turnover      int64 `json:"turnover"` // сумма проданных лотов, копейки
	Online        int64 `json:"online"`
}

func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	var s Stats
	err := h.pool.QueryRow(r.Context(), `
		SELECT
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM users WHERE blocked_at IS NOT NULL),
			(SELECT count(*) FROM lots WHERE status = 'active'),
			(SELECT count(*) FROM lots WHERE status = 'finished'),
			(SELECT count(*) FROM lots WHERE status = 'finished' AND winner_id IS NOT NULL),
			(SELECT count(*) FROM lots WHERE status = 'cancelled'),
			(SELECT count(*) FROM bids),
			(SELECT count(*) FROM bids WHERE created_at > now() - interval '24 hours'),
			(SELECT COALESCE(sum(final_price), 0) FROM lots WHERE status = 'finished')`,
	).Scan(&s.Users, &s.BlockedUsers, &s.ActiveLots, &s.FinishedLots, &s.SoldLots, &s.CancelledLots,
		&s.Bids, &s.Bids24h, &s.Turnover)
	if err != nil {
		h.internal(w, err)
		return
	}
	s.Online = h.online()
	httpx.JSON(w, http.StatusOK, s)
}

type FeedBid struct {
	ID        int64     `json:"id"`
	LotID     int64     `json:"lotId"`
	LotTitle  string    `json:"lotTitle"`
	UserID    int64     `json:"userId"`
	UserName  string    `json:"userName"`
	Amount    int64     `json:"amount"`
	CreatedAt time.Time `json:"createdAt"`
}

// RecentBids — последние ставки по всем лотам (лента на вкладке «Обзор»).
func (h *Handlers) RecentBids(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT b.id, b.lot_id, l.title, b.user_id, u.display_name, b.amount, b.created_at
		FROM bids b
		JOIN lots l  ON l.id = b.lot_id
		JOIN users u ON u.id = b.user_id
		ORDER BY b.id DESC
		LIMIT 30`)
	if err != nil {
		h.internal(w, err)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[FeedBid])
	if err != nil {
		h.internal(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, items)
}

// Lots — все лоты, включая снятые, с поиском и фильтром по статусу.
func (h *Handlers) Lots(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	status := q.Get("status")
	if status != "active" && status != "finished" && status != "cancelled" {
		status = ""
	}
	items, total, err := h.lots.List(r.Context(), lots.ListFilter{
		Status: status,
		Query:  q.Get("q"),
		Sort:   "newest",
		Limit:  100,
	})
	if err != nil {
		h.internal(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

type cancelRequest struct {
	Reason string `json:"reason"`
}

func (h *Handlers) CancelLot(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(w, r, "id")
	if !ok {
		return
	}
	var req cancelRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if n := len([]rune(req.Reason)); n < 3 || n > 500 {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_reason", "Укажите причину (от 3 до 500 символов)")
		return
	}

	c, err := h.lots.Cancel(r.Context(), id, req.Reason)
	switch {
	case errors.Is(err, lots.ErrLotNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "Лот не найден")
		return
	case errors.Is(err, lots.ErrAuctionClosed):
		httpx.Error(w, http.StatusConflict, "not_active", "Снять можно только лот, по которому идут торги")
		return
	case err != nil:
		h.internal(w, err)
		return
	}

	admin, _ := auth.CurrentUser(r.Context())
	h.log.Info("lot cancelled by admin", "lot", id, "admin", admin.ID, "reason", req.Reason)
	lots.BroadcastCancelled(h.hub, c)
	httpx.JSON(w, http.StatusOK, c)
}

type UserRow struct {
	ID          int64      `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"displayName"`
	Role        string     `json:"role"`
	BlockedAt   *time.Time `json:"blockedAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	Lots        int        `json:"lots"`
	ActiveLots  int        `json:"activeLots"`
	Bids        int        `json:"bids"`
	Wins        int        `json:"wins"`
}

func (h *Handlers) Users(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	rows, err := h.pool.Query(r.Context(), `
		SELECT u.id, u.email, u.display_name, u.role, u.blocked_at, u.created_at,
		       (SELECT count(*) FROM lots l WHERE l.seller_id = u.id),
		       (SELECT count(*) FROM lots l WHERE l.seller_id = u.id AND l.status = 'active'),
		       (SELECT count(*) FROM bids b WHERE b.user_id = u.id),
		       (SELECT count(*) FROM lots l WHERE l.winner_id = u.id)
		FROM users u
		WHERE $1 = '' OR u.email ILIKE '%' || $1 || '%' OR u.display_name ILIKE '%' || $1 || '%'
		ORDER BY u.role = 'admin' DESC, u.id
		LIMIT 200`, q)
	if err != nil {
		h.internal(w, err)
		return
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[UserRow])
	if err != nil {
		h.internal(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, items)
}

type blockRequest struct {
	CancelLots bool `json:"cancelLots"` // заодно снять все активные лоты пользователя
}

func (h *Handlers) BlockUser(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(w, r, "id")
	if !ok {
		return
	}
	var req blockRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	admin, _ := auth.CurrentUser(r.Context())
	if id == admin.ID {
		httpx.Error(w, http.StatusUnprocessableEntity, "self_block", "Нельзя заблокировать самого себя")
		return
	}

	var role string
	err := h.pool.QueryRow(r.Context(), `
		UPDATE users SET blocked_at = COALESCE(blocked_at, now())
		WHERE id = $1 AND role <> 'admin'
		RETURNING role`, id).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, http.StatusUnprocessableEntity, "cannot_block", "Пользователь не найден или является администратором")
		return
	}
	if err != nil {
		h.internal(w, err)
		return
	}

	cancelled := []lots.Cancelled{}
	if req.CancelLots {
		cancelled, err = h.lots.CancelBySeller(r.Context(), id, "Продавец заблокирован администратором")
		if err != nil {
			h.internal(w, err)
			return
		}
		for _, c := range cancelled {
			lots.BroadcastCancelled(h.hub, c)
		}
	}

	h.log.Info("user blocked", "user", id, "admin", admin.ID, "cancelledLots", len(cancelled))
	httpx.JSON(w, http.StatusOK, map[string]any{"cancelledLots": len(cancelled)})
}

func (h *Handlers) UnblockUser(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(w, r, "id")
	if !ok {
		return
	}
	tag, err := h.pool.Exec(r.Context(), `UPDATE users SET blocked_at = NULL WHERE id = $1`, id)
	if err != nil {
		h.internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, http.StatusNotFound, "not_found", "Пользователь не найден")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) internal(w http.ResponseWriter, err error) {
	h.log.Error("admin request failed", "err", err)
	httpx.Error(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка")
}
