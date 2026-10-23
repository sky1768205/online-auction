// Package realtime — WebSocket-хаб с комнатами.
//
// Протокол (JSON-сообщения):
//
//	клиент → сервер: {"type":"subscribe","room":"lot:42"}
//	                 {"type":"unsubscribe","room":"lot:42"}
//	                 {"type":"ping"}
//	сервер → клиент: {"type":"welcome","data":{"serverTime":...}}
//	                 {"type":"pong","data":{"serverTime":...}}
//	                 {"type":"subscribed","room":"lot:42"}
//	                 {"type":"bid:new","room":"lot:42","data":{...}}
//	                 {"type":"lot:closed","room":"lot:42","data":{...}}
//	                 {"type":"lot:created","room":"catalog","data":{...}}
//	                 {"type":"presence","room":"lot:42","data":{"watching":17}}
//	                 {"type":"bid:outbid" | "lot:won" | "lot:sold","room":"user:7","data":{...}}
//
// Комнаты: "catalog" (все обновления цен для каталога), "lot:<id>" и
// персональная "user:<id>" — в неё сервер добавляет авторизованного клиента сам,
// подписаться на чужую комнату через subscribe нельзя.
package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

const (
	sendBuffer       = 64 // очередь исходящих сообщений на клиента
	writeTimeout     = 5 * time.Second
	pingInterval     = 25 * time.Second
	maxSubscriptions = 50
	maxMessageSize   = 4096
)

var roomRe = regexp.MustCompile(`^(catalog|lot:[1-9][0-9]{0,18})$`)

type Event struct {
	Type string `json:"type"`
	Room string `json:"room,omitempty"`
	Data any    `json:"data,omitempty"`
}

type Hub struct {
	mu      sync.RWMutex
	rooms   map[string]map[*client]struct{}
	clients atomic.Int64

	presence map[string]int // последнее разосланное число зрителей по комнатам лотов

	originPatterns []string
	identify       func(*http.Request) (int64, bool)
	log            *slog.Logger
}

// NewHub создаёт хаб. identify определяет пользователя по запросу (cookie с JWT);
// анонимные клиенты тоже допускаются — они просто не получают персональных событий.
func NewHub(originPatterns []string, identify func(*http.Request) (int64, bool), log *slog.Logger) *Hub {
	return &Hub{
		rooms:          make(map[string]map[*client]struct{}),
		presence:       make(map[string]int),
		originPatterns: originPatterns,
		identify:       identify,
		log:            log,
	}
}

func UserRoom(userID int64) string { return "user:" + strconv.FormatInt(userID, 10) }

// RunPresence раз в interval рассылает число зрителей в комнаты лотов, где оно изменилось.
// Пакетная рассылка вместо события на каждый вход/выход: при 1000 подключений
// это одно сообщение на клиента, а не миллион.
func (h *Hub) RunPresence(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		changed := map[string]int{}
		h.mu.Lock()
		for room := range h.presence {
			if _, alive := h.rooms[room]; !alive {
				delete(h.presence, room) // зрителей не осталось — рассылать некому
			}
		}
		for room, members := range h.rooms {
			if strings.HasPrefix(room, "lot:") && h.presence[room] != len(members) {
				h.presence[room] = len(members)
				changed[room] = len(members)
			}
		}
		h.mu.Unlock()

		for room, n := range changed {
			h.Broadcast(room, "presence", map[string]int{"watching": n})
		}
	}
}

func (h *Hub) Clients() int64 { return h.clients.Load() }

// Broadcast сериализует событие один раз и ставит его в очередь каждому подписчику.
// Не блокируется: медленный клиент с переполненной очередью отключается.
func (h *Hub) Broadcast(room, eventType string, data any) {
	msg, err := json.Marshal(Event{Type: eventType, Room: room, Data: data})
	if err != nil {
		h.log.Error("marshal event", "err", err)
		return
	}

	h.mu.RLock()
	targets := make([]*client, 0, len(h.rooms[room]))
	for c := range h.rooms[room] {
		targets = append(targets, c)
	}
	h.mu.RUnlock()

	for _, c := range targets {
		c.enqueue(msg)
	}
}

func (h *Hub) subscribe(c *client, room string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := c.rooms[room]; ok {
		return true
	}
	if len(c.rooms) >= maxSubscriptions {
		return false
	}
	h.join(c, room)
	return true
}

// watching — текущее число подписчиков комнаты.
func (h *Hub) watching(room string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms[room])
}

func (h *Hub) join(c *client, room string) {
	if h.rooms[room] == nil {
		h.rooms[room] = make(map[*client]struct{})
	}
	h.rooms[room][c] = struct{}{}
	c.rooms[room] = struct{}{}
}

func (h *Hub) unsubscribe(c *client, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.leave(c, room)
}

func (h *Hub) leave(c *client, room string) {
	delete(c.rooms, room)
	if members := h.rooms[room]; members != nil {
		delete(members, c)
		if len(members) == 0 {
			delete(h.rooms, room)
		}
	}
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for room := range c.rooms {
		h.leave(c, room)
	}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.originPatterns})
	if err != nil {
		return // Accept уже записал ответ с ошибкой
	}
	conn.SetReadLimit(maxMessageSize)

	ctx, cancel := context.WithCancel(r.Context())
	c := &client{
		conn:   conn,
		send:   make(chan []byte, sendBuffer),
		rooms:  make(map[string]struct{}),
		ctx:    ctx,
		cancel: cancel,
	}

	h.clients.Add(1)
	defer func() {
		h.remove(c)
		h.clients.Add(-1)
		cancel()
		conn.CloseNow()
	}()

	welcome := map[string]any{"serverTime": time.Now()}
	if userID, ok := h.identify(r); ok {
		h.mu.Lock()
		h.join(c, UserRoom(userID))
		h.mu.Unlock()
		welcome["userId"] = userID
	}
	c.sendEvent(Event{Type: "welcome", Data: welcome})
	go c.writeLoop()
	c.readLoop(h)
}

func serverTime() map[string]time.Time {
	return map[string]time.Time{"serverTime": time.Now()}
}

type client struct {
	conn   *websocket.Conn
	send   chan []byte
	rooms  map[string]struct{} // защищено Hub.mu
	ctx    context.Context
	cancel context.CancelFunc
}

func (c *client) enqueue(msg []byte) {
	select {
	case c.send <- msg:
	case <-c.ctx.Done():
	default:
		// Клиент не успевает читать — отключаем, чтобы не тормозить остальных.
		c.cancel()
	}
}

func (c *client) sendEvent(e Event) {
	if msg, err := json.Marshal(e); err == nil {
		c.enqueue(msg)
	}
}

func (c *client) readLoop(h *Hub) {
	for {
		_, data, err := c.conn.Read(c.ctx)
		if err != nil {
			return
		}
		var in struct {
			Type string `json:"type"`
			Room string `json:"room"`
		}
		if json.Unmarshal(data, &in) != nil {
			c.sendEvent(Event{Type: "error", Data: "bad_message"})
			continue
		}
		switch in.Type {
		case "subscribe":
			if !roomRe.MatchString(in.Room) {
				c.sendEvent(Event{Type: "error", Room: in.Room, Data: "bad_room"})
			} else if !h.subscribe(c, in.Room) {
				c.sendEvent(Event{Type: "error", Room: in.Room, Data: "too_many_subscriptions"})
			} else {
				c.sendEvent(Event{Type: "subscribed", Room: in.Room})
				if strings.HasPrefix(in.Room, "lot:") {
					c.sendEvent(Event{Type: "presence", Room: in.Room, Data: map[string]int{"watching": h.watching(in.Room)}})
				}
			}
		case "unsubscribe":
			h.unsubscribe(c, in.Room)
		case "ping":
			c.sendEvent(Event{Type: "pong", Data: serverTime()})
		default:
			c.sendEvent(Event{Type: "error", Data: "unknown_type"})
		}
	}
}

func (c *client) writeLoop() {
	ping := time.NewTicker(pingInterval)
	defer ping.Stop()
	defer c.cancel()

	for {
		select {
		case <-c.ctx.Done():
			return
		case msg := <-c.send:
			ctx, cancel := context.WithTimeout(c.ctx, writeTimeout)
			err := c.conn.Write(ctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				return
			}
		case <-ping.C:
			ctx, cancel := context.WithTimeout(c.ctx, writeTimeout)
			err := c.conn.Ping(ctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
