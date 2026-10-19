// Copyright (c) 2026 Qiang Li. All rights reserved.
// Ported from kg-ui under explicit operator authorization for Bashy.
// See ATTRIBUTION.md and LICENSE for licensing information.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SSEHub manages connected Server-Sent Events clients and broadcasts events.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[chan []byte]bool
}

// NewSSEHub creates a new SSEHub.
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[chan []byte]bool),
	}
}

// Subscribe registers a new client channel.
func (h *SSEHub) Subscribe() chan []byte {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[ch] = true
	return ch
}

// Unsubscribe removes and closes a client channel.
func (h *SSEHub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
}

// Broadcast sends an event object to all active SSE clients.
func (h *SSEHub) Broadcast(event any) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	payload := fmt.Sprintf("data: %s\n\n", data)
	raw := []byte(payload)

	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.clients {
		select {
		case ch <- raw:
		default:
			// client buffer full, drop to prevent blocking
		}
	}
}

// StartHeartbeat sends periodic ping events to keep SSE connections alive.
func (h *SSEHub) StartHeartbeat(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case t := <-ticker.C:
				h.Broadcast(map[string]any{
					"type":      "heartbeat",
					"timestamp": t.UTC().Format(time.RFC3339),
				})
			}
		}
	}()
}

// ServeHTTP handles the /events SSE stream endpoint.
func (h *SSEHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := h.Subscribe()
	defer h.Unsubscribe(ch)

	// Send initial connection event
	initMsg := fmt.Sprintf("data: {\"type\":\"init\",\"status\":\"ready\",\"timestamp\":\"%s\"}\n\n",
		time.Now().UTC().Format(time.RFC3339))
	_, _ = w.Write([]byte(initMsg))
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if _, err := w.Write(msg); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
