package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/dark/x-mail/api/internal/auth"
	"github.com/dark/x-mail/api/internal/httpx"
)

const (
	// writeTimeout bounds a single frame write, so one stalled browser cannot
	// hold a goroutine forever.
	writeTimeout = 10 * time.Second
	// pingInterval keeps the connection alive through proxies that drop idle
	// sockets, and detects a client that has gone away without closing.
	pingInterval = 30 * time.Second
	// sendBuffer is how far a slow client may fall behind before it is dropped.
	sendBuffer = 32
)

// Hub holds the open sockets for this process and fans Redis events out to
// them.
type Hub struct {
	rdb    *redis.Client
	logger *slog.Logger
	// allowedOrigins are the hosts a browser may open a socket from. The
	// upgrade's origin check compares Origin against Host by default, which
	// fails behind any proxy that rewrites Host — so the allowed set is stated
	// explicitly instead.
	allowedOrigins []string

	mu      sync.RWMutex
	clients map[uuid.UUID]map[*client]struct{}
}

type client struct {
	send chan []byte
	once sync.Once
}

func (c *client) close() {
	c.once.Do(func() { close(c.send) })
}

// NewHub builds the hub. allowedOrigins are full origins ("https://mail.example");
// only their host parts are compared.
func NewHub(rdb *redis.Client, logger *slog.Logger, allowedOrigins []string) *Hub {
	patterns := make([]string, 0, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		if parsed, err := url.Parse(origin); err == nil && parsed.Host != "" {
			patterns = append(patterns, parsed.Host)
		}
	}

	return &Hub{
		rdb:            rdb,
		logger:         logger,
		allowedOrigins: patterns,
		clients:        make(map[uuid.UUID]map[*client]struct{}),
	}
}

// Handler upgrades a request to a websocket.
//
// It is mounted behind the session middleware, so the cookie has already been
// verified and the socket belongs to a known user. Events are then scoped to
// that user's Redis channel, which is what keeps one person's mail off another
// person's socket.
func (h *Hub) Handler(w http.ResponseWriter, r *http.Request) {
	user, err := auth.MustUser(r.Context())
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: h.allowedOrigins,
	})
	if err != nil {
		h.logger.WarnContext(r.Context(), "websocket upgrade failed", "error", err)
		return
	}

	c := &client{send: make(chan []byte, sendBuffer)}
	h.add(user.ID, c)
	defer h.remove(user.ID, c)

	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()

	// A read loop is required even though the client sends nothing: it is what
	// notices the socket closing and surfaces protocol errors.
	go func() {
		defer cancel()
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = conn.Close(websocket.StatusNormalClosure, "")
			return

		case payload, ok := <-c.send:
			if !ok {
				_ = conn.Close(websocket.StatusPolicyViolation, "too far behind")
				return
			}
			writeCtx, cancelWrite := context.WithTimeout(ctx, writeTimeout)
			err := conn.Write(writeCtx, websocket.MessageText, payload)
			cancelWrite()
			if err != nil {
				_ = conn.Close(websocket.StatusInternalError, "write failed")
				return
			}

		case <-ticker.C:
			pingCtx, cancelPing := context.WithTimeout(ctx, writeTimeout)
			err := conn.Ping(pingCtx)
			cancelPing()
			if err != nil {
				_ = conn.Close(websocket.StatusNormalClosure, "")
				return
			}
		}
	}
}

func (h *Hub) add(userID uuid.UUID, c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[*client]struct{})
	}
	h.clients[userID][c] = struct{}{}
}

func (h *Hub) remove(userID uuid.UUID, c *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if sockets, ok := h.clients[userID]; ok {
		delete(sockets, c)
		if len(sockets) == 0 {
			delete(h.clients, userID)
		}
	}
	c.close()
}

// deliver pushes a payload to every socket this user has open — several tabs,
// or a phone and a laptop.
//
// A client whose buffer is full is dropped rather than waited on: blocking here
// would let one stalled browser hold up the fan-out for everyone.
func (h *Hub) deliver(userID uuid.UUID, payload []byte) {
	h.mu.RLock()
	sockets := make([]*client, 0, len(h.clients[userID]))
	for c := range h.clients[userID] {
		sockets = append(sockets, c)
	}
	h.mu.RUnlock()

	for _, c := range sockets {
		select {
		case c.send <- payload:
		default:
			h.logger.Warn("dropping a websocket client that fell behind", "user_id", userID)
			c.close()
		}
	}
}

// Run subscribes to every user channel and fans messages out to local sockets.
// It blocks until the context is cancelled.
func (h *Hub) Run(ctx context.Context) error {
	// One pattern subscription rather than one per connected user: users come
	// and go constantly, and resubscribing on each would be needless churn.
	sub := h.rdb.PSubscribe(ctx, "realtime:user:*")
	defer func() { _ = sub.Close() }()

	messages := sub.Channel()
	h.logger.Info("realtime hub subscribed")

	for {
		select {
		case <-ctx.Done():
			return nil

		case message, ok := <-messages:
			if !ok {
				return errors.New("realtime: subscription closed")
			}

			userID, err := userFromChannel(message.Channel)
			if err != nil {
				continue
			}

			// Validate before fanning out, so a malformed publish cannot send
			// junk down a socket.
			var probe map[string]any
			if err := json.Unmarshal([]byte(message.Payload), &probe); err != nil {
				h.logger.Warn("discarding an unparseable realtime payload", "error", err)
				continue
			}
			h.deliver(userID, []byte(message.Payload))
		}
	}
}

func userFromChannel(channel string) (uuid.UUID, error) {
	const prefix = "realtime:user:"
	if len(channel) <= len(prefix) {
		return uuid.Nil, errors.New("realtime: unexpected channel")
	}
	return uuid.Parse(channel[len(prefix):])
}

// Connections reports how many sockets are open, for metrics and tests.
func (h *Hub) Connections() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	total := 0
	for _, sockets := range h.clients {
		total += len(sockets)
	}
	return total
}
