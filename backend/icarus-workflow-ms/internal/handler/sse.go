package handler

import (
	"fmt"
	"net/http"
	"sync"
)

// SSEClient represents a connected SSE subscriber.
type SSEClient struct {
	UserID  string
	Channel chan string
}

// SSEBroker manages all active SSE connections in memory.
// For each incoming step action, we push a message to the relevant user's channel.
type SSEBroker struct {
	mu      sync.RWMutex
	clients map[string][]chan string // userID → list of open connections
}

func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		clients: make(map[string][]chan string),
	}
}

// Register adds a new channel for a user.
func (b *SSEBroker) Register(userID string) chan string {
	ch := make(chan string, 8)
	b.mu.Lock()
	b.clients[userID] = append(b.clients[userID], ch)
	b.mu.Unlock()
	return ch
}

// Deregister removes a channel when the client disconnects.
func (b *SSEBroker) Deregister(userID string, ch chan string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	list := b.clients[userID]
	newList := list[:0]
	for _, c := range list {
		if c != ch {
			newList = append(newList, c)
		}
	}
	if len(newList) == 0 {
		delete(b.clients, userID)
	} else {
		b.clients[userID] = newList
	}
}

// Publish sends an SSE message to all open connections for a user.
func (b *SSEBroker) Publish(userID, eventType, payload string) {
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, payload)
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.clients[userID] {
		select {
		case ch <- msg:
		default: // Drop if channel is full (slow consumer)
		}
	}
}

// SSEHandler GET /api/v1/workflow/events
// Holds the connection open and streams events as they arrive.
// On connect, replays any unread notifications from the DB first.
func (s *HandlerServer) SSEHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)
	userID := claims.Subject

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Nginx: disable buffering

	// Replay unread notifications from DB (reconnect support)
	unread, _ := s.Repo.GetUnreadSSENotifications(userID)
	for _, n := range unread {
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", n.EventType, n.Payload)
	}
	if len(unread) > 0 {
		_ = s.Repo.MarkNotificationsRead(userID)
		flusher.Flush()
	}

	// Register this connection
	ch := s.SSEBroker.Register(userID)
	defer s.SSEBroker.Deregister(userID, ch)

	// Send a heartbeat comment to keep the connection alive
	_, _ = fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	for {
		select {
		case msg := <-ch:
			_, _ = fmt.Fprint(w, msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
