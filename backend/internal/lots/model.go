package lots

import (
	"errors"
	"fmt"
	"time"
)

type Category struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Image struct {
	ID       int64  `json:"id"`
	URL      string `json:"url"`
	Position int    `json:"position"`
}

type Lot struct {
	ID           int64      `json:"id"`
	SellerID     int64      `json:"sellerId"`
	SellerName   string     `json:"sellerName"`
	CategoryID   int64      `json:"categoryId"`
	CategorySlug string     `json:"categorySlug"`
	CategoryName string     `json:"categoryName"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	StartPrice   int64      `json:"startPrice"`
	MinStep      int64      `json:"minStep"`
	CurrentPrice *int64     `json:"currentPrice"`
	LeaderID     *int64     `json:"leaderId"`
	LeaderName   *string    `json:"leaderName"`
	BidCount     int        `json:"bidCount"`
	MinNextBid   int64      `json:"minNextBid"`
	StartsAt     time.Time  `json:"startsAt"`
	EndsAt       time.Time  `json:"endsAt"`
	Status       string     `json:"status"`
	WinnerID     *int64     `json:"winnerId"`
	WinnerName   *string    `json:"winnerName"`
	FinalPrice   *int64     `json:"finalPrice"`
	ClosedAt     *time.Time `json:"closedAt"`
	CancelReason *string    `json:"cancelReason"`
	CoverURL     *string    `json:"coverUrl"`
	Images       []Image    `json:"images,omitempty"`
}

type Bid struct {
	ID        int64     `json:"id"`
	LotID     int64     `json:"lotId"`
	UserID    int64     `json:"userId"`
	UserName  string    `json:"userName"`
	Amount    int64     `json:"amount"`
	CreatedAt time.Time `json:"createdAt"`
}

// BidPlaced — результат принятой ставки; рассылается всем участникам как событие bid:new.
type BidPlaced struct {
	LotID        int64     `json:"lotId"`
	Bid          Bid       `json:"bid"`
	CurrentPrice int64     `json:"currentPrice"`
	LeaderID     int64     `json:"leaderId"`
	LeaderName   string    `json:"leaderName"`
	BidCount     int       `json:"bidCount"`
	MinNextBid   int64     `json:"minNextBid"`
	EndsAt       time.Time `json:"endsAt"`
	Extended     bool      `json:"extended"` // таймер продлён защитой от «снайперов»

	LotTitle     string `json:"lotTitle"`
	PrevLeaderID *int64 `json:"-"` // кого перебили — ему уходит персональное уведомление
}

// Participation — лот в «Моих торгах» вместе с максимальной ставкой пользователя.
type Participation struct {
	Lot      Lot   `json:"lot"`
	MyMaxBid int64 `json:"myMaxBid"`
}

// Cancelled — лот снят с торгов администратором; рассылается как событие lot:cancelled.
type Cancelled struct {
	LotID    int64     `json:"lotId"`
	LotTitle string    `json:"lotTitle"`
	SellerID int64     `json:"sellerId"`
	Reason   string    `json:"reason"`
	ClosedAt time.Time `json:"closedAt"`
	LeaderID *int64    `json:"-"` // лидеру торгов уходит персональное уведомление
}

// MinNextBid — минимально допустимая следующая ставка:
// стартовая цена, если ставок не было, иначе текущая + шаг.
func MinNextBid(startPrice, minStep int64, current *int64) int64 {
	if current == nil {
		return startPrice
	}
	return *current + minStep
}

var (
	ErrLotNotFound    = errors.New("lot not found")
	ErrAuctionClosed  = errors.New("auction closed")
	ErrLotCancelled   = errors.New("lot cancelled")
	ErrNotStarted     = errors.New("auction not started")
	ErrOwnLot         = errors.New("cannot bid on own lot")
	ErrAlreadyLeading = errors.New("already leading")
)

type BidTooLowError struct {
	MinNextBid int64
}

func (e *BidTooLowError) Error() string {
	return fmt.Sprintf("bid too low, minimum is %d", e.MinNextBid)
}
