package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/KB-perByte/hiveshare/internal/models"
)

const channelPrefix = "hiveshare:"

// Hub manages SSE connections and fans out events from Redis pub/sub.
// One Redis subscription is held per hiveshare per server instance; local
// SSE clients share it via the in-memory clients map.
type Hub struct {
	rdb     *redis.Client
	mu      sync.RWMutex
	clients map[uuid.UUID]map[chan []byte]struct{} // hiveshareID → set of client channels
	subs    map[uuid.UUID]*hiveshareSub            // hiveshareID → shared Redis subscription
}

type hiveshareSub struct {
	cancel context.CancelFunc
	sub    *redis.PubSub
}

func NewHub(rdb *redis.Client) *Hub {
	return &Hub{
		rdb:     rdb,
		clients: make(map[uuid.UUID]map[chan []byte]struct{}),
		subs:    make(map[uuid.UUID]*hiveshareSub),
	}
}

// Publish sends an event to all clients in the hiveshare via Redis.
func (h *Hub) Publish(ctx context.Context, ev models.StreamEvent) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	channel := channelPrefix + ev.HiveshareID.String() + ":events"
	return h.rdb.Publish(ctx, channel, data).Err()
}

// ServeSSE handles an SSE connection for a specific hiveshare.
func (h *Hub) ServeSSE(w http.ResponseWriter, r *http.Request, hiveshareID uuid.UUID) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Disable write deadline for this long-lived response if the server set one.
	if rc := http.NewResponseController(w); rc != nil {
		_ = rc.SetWriteDeadline(time.Time{})
	}

	ch := make(chan []byte, 32)
	h.subscribe(hiveshareID, ch)
	defer h.unsubscribe(hiveshareID, ch)

	ctx := r.Context()

	// send initial connected event
	fmt.Fprintf(w, "event: connected\ndata: {\"hiveshare_id\":\"%s\"}\n\n", hiveshareID)
	flusher.Flush()

	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-keepalive.C:
			// Comment frame keeps proxies / LBs from idle-closing the socket.
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case data, ok := <-ch:
			if !ok {
				return
			}
			var ev models.StreamEvent
			if err := json.Unmarshal(data, &ev); err != nil {
				slog.Error("realtime: unmarshal error", "err", err)
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, data)
			flusher.Flush()
		}
	}
}

func (h *Hub) subscribe(hsID uuid.UUID, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[hsID] == nil {
		h.clients[hsID] = make(map[chan []byte]struct{})
	}
	h.clients[hsID][ch] = struct{}{}

	// Start one Redis subscription for this hiveshare if this is the first local client.
	if _, ok := h.subs[hsID]; !ok {
		h.startRedisSubLocked(hsID)
	}
}

func (h *Hub) unsubscribe(hsID uuid.UUID, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[hsID], ch)
	if len(h.clients[hsID]) == 0 {
		delete(h.clients, hsID)
		if sub, ok := h.subs[hsID]; ok {
			sub.cancel()
			_ = sub.sub.Close()
			delete(h.subs, hsID)
		}
	}
}

// startRedisSubLocked must be called with h.mu held.
func (h *Hub) startRedisSubLocked(hsID uuid.UUID) {
	ctx, cancel := context.WithCancel(context.Background())
	channel := channelPrefix + hsID.String() + ":events"
	sub := h.rdb.Subscribe(ctx, channel)
	h.subs[hsID] = &hiveshareSub{cancel: cancel, sub: sub}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-sub.Channel():
				if !ok {
					return
				}
				h.fanOut(hsID, []byte(msg.Payload))
			}
		}
	}()
}

func (h *Hub) fanOut(hsID uuid.UUID, data []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients[hsID] {
		select {
		case ch <- data:
		default:
			// slow client: drop
		}
	}
}
