package events

import (
	"fmt"
	"net/http"
	"sync"
)

// Hub is a minimal in-memory broadcaster for Server-Sent Events.
// Used to push real-time bed availability updates to connected browsers.
type Hub struct {
	mu        sync.RWMutex
	clients   map[chan string]struct{}
	broadcast chan string
}

func NewHub() *Hub {
	h := &Hub{
		clients:   make(map[chan string]struct{}),
		broadcast: make(chan string, 32),
	}
	go h.run()
	return h
}

func (h *Hub) run() {
	for msg := range h.broadcast {
		h.mu.RLock()
		for c := range h.clients {
			select {
			case c <- msg:
			default:
				// slow client, drop
			}
		}
		h.mu.RUnlock()
	}
}

func (h *Hub) Publish(msg string) {
	select {
	case h.broadcast <- msg:
	default:
	}
}

// Subscribe returns a channel that will receive messages.
// Caller must unregister when done.
func (h *Hub) Subscribe() chan string {
	c := make(chan string, 8)
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

func (h *Hub) Unsubscribe(c chan string) {
	h.mu.Lock()
	delete(h.clients, c)
	close(c)
	h.mu.Unlock()
}

// ServeSSE is a http.HandlerFunc that streams events to the client.
func (h *Hub) ServeSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	c := h.Subscribe()
	defer h.Unsubscribe(c)

	// send a comment to keep connection alive in some proxies
	fmt.Fprintf(w, ": connected to Sigap bed events\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-c:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: bed_updated\ndata: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

// Bus is the default global hub for simplicity in this scaffold.
// In a larger app you would inject the hub.
var Bus = NewHub()
