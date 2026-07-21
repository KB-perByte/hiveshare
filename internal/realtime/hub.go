package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/sagpaul/hiveshare/internal/models"
)

const channelPrefix = "hiveshare:"

// Hub manages SSE connections and fans out events from Redis pub/sub.
type Hub struct {
	rdb     *redis.Client
	mu      sync.RWMutex
	clients map[uuid.UUID]map[chan []byte]struct{} // hiveshareID → set of client channels
}

func NewHub(rdb *redis.Client) *Hub {
	return &Hub{
		rdb:     rdb,
		clients: make(map[uuid.UUID]map[chan []byte]struct{}),
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

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	ch := make(chan []byte, 32)
	h.subscribe(hiveshareID, ch)
	defer h.unsubscribe(hiveshareID, ch)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// subscribe to Redis for this hiveshare
	channel := channelPrefix + hiveshareID.String() + ":events"
	sub := h.rdb.Subscribe(ctx, channel)
	defer sub.Close()

	go func() {
		for msg := range sub.Channel() {
			select {
			case ch <- []byte(msg.Payload):
			default:
				// slow client: drop
			}
		}
	}()

	// send initial connected event
	fmt.Fprintf(w, "event: connected\ndata: {\"hiveshare_id\":\"%s\"}\n\n", hiveshareID)
	flusher.Flush()

	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-ch:
			if !ok {
				return
			}
			var ev models.StreamEvent
			if err := json.Unmarshal(data, &ev); err != nil {
				log.Printf("realtime: unmarshal error: %v", err)
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
}

func (h *Hub) unsubscribe(hsID uuid.UUID, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[hsID], ch)
	if len(h.clients[hsID]) == 0 {
		delete(h.clients, hsID)
	}
}
