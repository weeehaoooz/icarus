package handler

import (
	"fmt"
	"icarus-workflow-ms/internal/models"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
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
	ch := make(chan string, 16)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.clients[userID] = append(b.clients[userID], ch)
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
// Recovers from panics and drops full channels gracefully.
func (b *SSEBroker) Publish(userID, eventType, payload string) {
	if userID == "" {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[SSEBroker] Recovered from panic in Publish: %v", r)
		}
	}()

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

// sendSSENotification creates a notification record in DB (if available) and publishes via SSE.
// Errors during DB write or SSE delivery are logged as warnings and will not fail the parent operation.
func (s *HandlerServer) sendSSENotification(userID, eventType, payload string) {
	if userID == "" {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Notification] Recovered from panic in sendSSENotification: %v", r)
		}
	}()

	if s.Repo != nil {
		if err := s.Repo.CreateSSENotification(&models.SSENotification{
			ID:        uuid.New().String(),
			UserID:    userID,
			EventType: eventType,
			Payload:   payload,
		}); err != nil {
			log.Printf("[Notification] Warning: failed to persist SSE notification for user '%s' (%s): %v", userID, eventType, err)
		}
	}

	if s.SSEBroker != nil {
		s.SSEBroker.Publish(userID, eventType, payload)
	}
}

// sendSSENotificationsToUsers sends a notification to multiple users, deduplicating user IDs.
func (s *HandlerServer) sendSSENotificationsToUsers(userIDs []string, eventType, payload string) {
	seen := make(map[string]bool, len(userIDs))
	for _, uid := range userIDs {
		if uid == "" || seen[uid] {
			continue
		}
		seen[uid] = true
		s.sendSSENotification(uid, eventType, payload)
	}
}

// SSEHandler GET /api/v1/workflow/events
// Holds the connection open and streams events as they arrive.
// On connect, replays any unread notifications from the DB first.
func (s *HandlerServer) SSEHandler(w http.ResponseWriter, r *http.Request) {
	claims := s.claimsFrom(r)
	userID := claims.Subject
	if userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Nginx: disable buffering

	// Replay unread notifications from DB (reconnect support)
	if s.Repo != nil {
		unread, err := s.Repo.GetUnreadSSENotifications(userID)
		if err != nil {
			log.Printf("[SSE] Warning: failed to fetch unread notifications for user '%s': %v", userID, err)
		} else {
			hasWriteError := false
			for _, n := range unread {
				if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", n.EventType, n.Payload); err != nil {
					hasWriteError = true
					break
				}
			}
			if !hasWriteError && len(unread) > 0 {
				if err := s.Repo.MarkNotificationsRead(userID); err != nil {
					log.Printf("[SSE] Warning: failed to mark notifications as read for user '%s': %v", userID, err)
				}
				flusher.Flush()
			}
			if hasWriteError {
				return
			}
		}
	}

	// Register this connection
	ch := s.SSEBroker.Register(userID)
	defer s.SSEBroker.Deregister(userID, ch)

	// Send initial connected comment
	if _, err := fmt.Fprintf(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	// Keep-alive ticker to prevent intermediate proxies from timing out
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if _, err := fmt.Fprint(w, msg); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := fmt.Fprintf(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
