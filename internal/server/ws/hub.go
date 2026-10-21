package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"todoapp/internal/models"
)

// Client represents an active WebSocket connection.
type Client struct {
	Hub      *Hub
	UserID   int64
	Username string
	Conn     *websocket.Conn
	Send     chan []byte
}

// Hub manages active WebSocket connections, presence, and room broadcasts.
type Hub struct {
	mu          sync.RWMutex
	clients     map[*Client]bool
	userClients map[int64]map[*Client]bool
	logger      *slog.Logger
	membersFunc func(ctx context.Context, roomID int64) ([]int64, error)
}

// NewHub initializes a new Hub.
func NewHub(logger *slog.Logger, membersFunc func(ctx context.Context, roomID int64) ([]int64, error)) *Hub {
	if logger == nil {
		logger = slog.Default()
	}
	return &Hub{
		clients:     make(map[*Client]bool),
		userClients: make(map[int64]map[*Client]bool),
		logger:      logger,
		membersFunc: membersFunc,
	}
}

// Register registers a new client connection.
func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	h.clients[c] = true
	if h.userClients[c.UserID] == nil {
		h.userClients[c.UserID] = make(map[*Client]bool)
	}
	wasOnline := len(h.userClients[c.UserID]) > 0
	h.userClients[c.UserID][c] = true
	h.mu.Unlock()

	// If this is the user's first active connection, broadcast presence
	if !wasOnline {
		h.BroadcastAll(models.WSEvent{
			Type: "presence",
			Payload: models.WSPresence{
				UserID:   c.UserID,
				Username: c.Username,
				Online:   true,
			},
		})
	}
}

// Unregister disconnects and cleans up a client.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; !ok {
		h.mu.Unlock()
		return
	}
	delete(h.clients, c)
	close(c.Send)

	if userConns, ok := h.userClients[c.UserID]; ok {
		delete(userConns, c)
		if len(userConns) == 0 {
			delete(h.userClients, c.UserID)
		}
	}
	isStillOnline := len(h.userClients[c.UserID]) > 0
	h.mu.Unlock()

	if !isStillOnline {
		h.BroadcastAll(models.WSEvent{
			Type: "presence",
			Payload: models.WSPresence{
				UserID:   c.UserID,
				Username: c.Username,
				Online:   false,
			},
		})
	}
}

// IsOnline reports whether a user currently has an active connection.
func (h *Hub) IsOnline(userID int64) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.userClients[userID]) > 0
}

// BroadcastToRoom dispatches an event to all connected members of a room.
func (h *Hub) BroadcastToRoom(ctx context.Context, roomID int64, event models.WSEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	var memberIDs []int64
	if h.membersFunc != nil {
		memberIDs, err = h.membersFunc(ctx, roomID)
		if err != nil {
			return err
		}
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, uID := range memberIDs {
		if conns, ok := h.userClients[uID]; ok {
			for c := range conns {
				select {
				case c.Send <- data:
				default:
					h.logger.Warn("client send buffer full, dropping event", "user_id", uID)
				}
			}
		}
	}
	return nil
}

// BroadcastToUser sends an event to all connections of a specific user.
func (h *Hub) BroadcastToUser(userID int64, event models.WSEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if conns, ok := h.userClients[userID]; ok {
		for c := range conns {
			select {
			case c.Send <- data:
			default:
			}
		}
	}
	return nil
}

// BroadcastAll sends an event to every connected client.
func (h *Hub) BroadcastAll(event models.WSEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for c := range h.clients {
		select {
		case c.Send <- data:
		default:
		}
	}
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

// WritePump pumps messages from the hub to the websocket connection.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Drain queued messages
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}
			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// ReadPump pumps messages from the websocket connection to the hub.
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister(c)
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(65536)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			break
		}
		// Handle incoming client messages (e.g. heartbeat ping)
		var event models.WSEvent
		if err := json.Unmarshal(message, &event); err == nil && event.Type == "ping" {
			pong, _ := json.Marshal(models.WSEvent{Type: "pong"})
			select {
			case c.Send <- pong:
			default:
			}
		}
	}
}
