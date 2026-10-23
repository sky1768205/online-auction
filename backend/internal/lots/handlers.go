package lots

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"lot/internal/auth"
	"lot/internal/httpx"
	"lot/internal/realtime"
)

// Broadcaster рассылает события подписчикам комнаты (реализован в realtime.Hub).
type Broadcaster interface {
	Broadcast(room, eventType string, data any)
}

func LotRoom(id int64) string { return "lot:" + strconv.FormatInt(id, 10) }

const CatalogRoom = "catalog"

type Handlers struct {
	repo *Repo
	hub  Broadcaster
	log  *slog.Logger
}

func NewHandlers(repo *Repo, hub Broadcaster, log *slog.Logger) *Handlers {
	return &Handlers{repo: repo, hub: hub, log: log}
}

func (h *Handlers) Categories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.repo.Categories(r.Context())
	if err != nil {
		h.internal(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, cats)
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := ListFilter{
		Category: q.Get("category"),
		Status:   q.Get("status"),
		Query:    q.Get("q"),
		Sort:     q.Get("sort"),
		Limit:    clamp(atoi(q.Get("limit"), 24), 1, 100),
	}
	if f.Status != "finished" {
		f.Status = "active"
	}
	if f.Sort == "" {
		f.Sort = "ending"
	}
	page := max(atoi(q.Get("page"), 1), 1)
	f.Offset = (page - 1) * f.Limit

	items, total, err := h.repo.List(r.Context(), f)
	if err != nil {
		h.internal(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"total":      total,
		"page":       page,
		"limit":      f.Limit,
		"serverTime": time.Now(),
	})
}

func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(w, r, "id")
	if !ok {
		return
	}
	lot, err := h.repo.Get(r.Context(), id)
	if errors.Is(err, ErrLotNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found", "Лот не найден")
		return
	}
	if err != nil {
		h.internal(w, err)
		return
	}
	// serverTime нужен клиенту, чтобы таймер не зависел от неточных часов пользователя.
	httpx.JSON(w, http.StatusOK, map[string]any{"lot": lot, "serverTime": time.Now()})
}

func (h *Handlers) Bids(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.IDParam(w, r, "id")
	if !ok {
		return
	}
	bids, err := h.repo.Bids(r.Context(), id, clamp(atoi(r.URL.Query().Get("limit"), 50), 1, 1000))
	if err != nil {
		h.internal(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, bids)
}

type createRequest struct {
	Title           string     `json:"title"`
	Description     string     `json:"description"`
	CategoryID      int64      `json:"categoryId"`
	StartPrice      int64      `json:"startPrice"` // в копейках
	MinStep         int64      `json:"minStep"`    // в копейках
	DurationMinutes int        `json:"durationMinutes,omitempty"`
	EndsAt          *time.Time `json:"endsAt,omitempty"`
	Images          []string   `json:"images"`
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	var req createRequest
	if !httpx.Decode(w, r, &req) {
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	endsAt := time.Now().Add(time.Duration(req.DurationMinutes) * time.Minute)
	if req.EndsAt != nil {
		endsAt = *req.EndsAt
	}

	fail := func(code, msg string) { httpx.Error(w, http.StatusUnprocessableEntity, code, msg) }
	switch n := len([]rune(req.Title)); {
	case n < 3 || n > 200:
		fail("invalid_title", "Название должно быть от 3 до 200 символов")
		return
	case len([]rune(req.Description)) > 10000:
		fail("invalid_description", "Описание слишком длинное")
		return
	case req.StartPrice <= 0:
		fail("invalid_start_price", "Стартовая цена должна быть больше нуля")
		return
	case req.MinStep <= 0:
		fail("invalid_min_step", "Шаг торгов должен быть больше нуля")
		return
	case time.Until(endsAt) < 10*time.Second || time.Until(endsAt) > 30*24*time.Hour:
		fail("invalid_duration", "Длительность торгов — от 10 секунд до 30 дней")
		return
	case len(req.Images) > 10:
		fail("too_many_images", "Не больше 10 изображений")
		return
	}
	for _, u := range req.Images {
		if !strings.HasPrefix(u, "/uploads/") && !strings.HasPrefix(u, "https://") {
			fail("invalid_image", "Некорректная ссылка на изображение")
			return
		}
	}

	id, err := h.repo.Create(r.Context(), CreateLot{
		SellerID: userID, CategoryID: req.CategoryID,
		Title: req.Title, Description: req.Description,
		StartPrice: req.StartPrice, MinStep: req.MinStep,
		EndsAt: endsAt, Images: req.Images,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23503" {
		fail("invalid_category", "Категория не найдена")
		return
	}
	if err != nil {
		h.internal(w, err)
		return
	}

	lot, err := h.repo.Get(r.Context(), id)
	if err != nil {
		h.internal(w, err)
		return
	}
	h.hub.Broadcast(CatalogRoom, "lot:created", lot)
	httpx.JSON(w, http.StatusCreated, lot)
}

type bidRequest struct {
	Amount int64 `json:"amount"` // в копейках
}

func (h *Handlers) PlaceBid(w http.ResponseWriter, r *http.Request) {
	lotID, ok := httpx.IDParam(w, r, "id")
	if !ok {
		return
	}
	userID, _ := auth.UserID(r.Context())
	var req bidRequest
	if !httpx.Decode(w, r, &req) {
		return
	}
	if req.Amount <= 0 {
		httpx.Error(w, http.StatusUnprocessableEntity, "invalid_amount", "Сумма ставки должна быть больше нуля")
		return
	}

	res, err := h.repo.PlaceBid(r.Context(), lotID, userID, req.Amount)
	var tooLow *BidTooLowError
	switch {
	case err == nil:
	case errors.Is(err, ErrLotNotFound):
		httpx.Error(w, http.StatusNotFound, "not_found", "Лот не найден")
		return
	case errors.Is(err, ErrLotCancelled):
		httpx.Error(w, http.StatusConflict, "lot_cancelled", "Лот снят с торгов")
		return
	case errors.Is(err, ErrAuctionClosed):
		httpx.Error(w, http.StatusConflict, "auction_closed", "Торги по лоту завершены")
		return
	case errors.Is(err, ErrNotStarted):
		httpx.Error(w, http.StatusConflict, "auction_not_started", "Торги ещё не начались")
		return
	case errors.Is(err, ErrOwnLot):
		httpx.Error(w, http.StatusForbidden, "own_lot", "Нельзя делать ставки на свой лот")
		return
	case errors.Is(err, ErrAlreadyLeading):
		httpx.Error(w, http.StatusConflict, "already_leading", "Ваша ставка уже лидирует")
		return
	case errors.As(err, &tooLow):
		httpx.JSON(w, http.StatusUnprocessableEntity, httpx.ErrorBody{
			Error:   "bid_too_low",
			Message: "Ставка должна быть не меньше текущей + шаг торгов",
			Details: map[string]int64{"minNextBid": tooLow.MinNextBid},
		})
		return
	default:
		h.internal(w, err)
		return
	}

	// Рассылаем только после COMMIT — клиенты никогда не увидят откатившуюся ставку.
	h.hub.Broadcast(LotRoom(lotID), "bid:new", res)
	h.hub.Broadcast(CatalogRoom, "bid:new", res)
	if res.PrevLeaderID != nil {
		h.hub.Broadcast(realtime.UserRoom(*res.PrevLeaderID), "bid:outbid", map[string]any{
			"lotId":        lotID,
			"lotTitle":     res.LotTitle,
			"currentPrice": res.CurrentPrice,
			"minNextBid":   res.MinNextBid,
			"leaderName":   res.LeaderName,
			"endsAt":       res.EndsAt,
		})
	}
	httpx.JSON(w, http.StatusCreated, res)
}

// MyBids — «Мои торги»: лоты, где пользователь делал ставки.
func (h *Handlers) MyBids(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	items, err := h.repo.UserBids(r.Context(), userID)
	if err != nil {
		h.internal(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "serverTime": time.Now()})
}

// MyLots — лоты, выставленные пользователем.
func (h *Handlers) MyLots(w http.ResponseWriter, r *http.Request) {
	userID, _ := auth.UserID(r.Context())
	items, err := h.repo.UserLots(r.Context(), userID)
	if err != nil {
		h.internal(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "serverTime": time.Now()})
}

func (h *Handlers) internal(w http.ResponseWriter, err error) {
	h.log.Error("request failed", "err", err)
	httpx.Error(w, http.StatusInternalServerError, "internal", "Внутренняя ошибка")
}

func atoi(s string, def int) int {
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return def
}

func clamp(v, lo, hi int) int { return min(max(v, lo), hi) }

// BroadcastCancelled рассылает снятие лота: всем на странице лота и в каталоге,
// а продавцу и лидеру торгов — персональное уведомление.
func BroadcastCancelled(hub Broadcaster, c Cancelled) {
	hub.Broadcast(LotRoom(c.LotID), "lot:cancelled", c)
	hub.Broadcast(CatalogRoom, "lot:cancelled", c)
	hub.Broadcast(realtime.UserRoom(c.SellerID), "lot:cancelled", c)
	if c.LeaderID != nil && *c.LeaderID != c.SellerID {
		hub.Broadcast(realtime.UserRoom(*c.LeaderID), "lot:cancelled", c)
	}
}
