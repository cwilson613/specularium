package hub

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Client represents a connected SSE client
type Client struct {
	id     string
	events chan []byte
	done   chan struct{}
}

// Hub manages SSE client connections
type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]struct{}
	register   chan *Client
	unregister chan *Client
	broadcast  chan interface{}
}

// New creates a new Hub
func New() *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan interface{}, 256),
	}
}

// Run starts the hub's event loop
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = struct{}{}
			h.mu.Unlock()
			log.Printf("SSE client connected: %s (total: %d)", client.id, len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.events)
			}
			h.mu.Unlock()
			log.Printf("SSE client disconnected: %s (total: %d)", client.id, len(h.clients))

		case event := <-h.broadcast:
			data, err := json.Marshal(event)
			if err != nil {
				log.Printf("Failed to marshal event: %v", err)
				continue
			}

			msg := fmt.Sprintf("data: %s\n\n", data)

			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.events <- []byte(msg):
				default:
					// Client is slow, skip this message
					log.Printf("SSE client %s is slow, skipping message", client.id)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast sends an event to all connected clients
func (h *Hub) Broadcast(event interface{}) {
	select {
	case h.broadcast <- event:
	default:
		log.Println("Broadcast channel full, dropping event")
	}
}

// ClientCount returns the number of connected clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ServeHTTP handles SSE connections
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if client supports SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Create client
	client := &Client{
		id:     fmt.Sprintf("%d", time.Now().UnixNano()),
		events: make(chan []byte, 64),
		done:   make(chan struct{}),
	}

	// Register client
	h.register <- client

	// Ensure cleanup on disconnect
	defer func() {
		h.unregister <- client
	}()

	// Send initial connection message
	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	// Keep-alive ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Event loop
	for {
		select {
		case msg, ok := <-client.events:
			if !ok {
				return
			}
			if _, err := w.Write(msg); err != nil {
				return
			}
			flusher.Flush()

		case <-ticker.C:
			// Send keep-alive comment
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()

		case <-r.Context().Done():
			return
		}
	}
}
