package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"todoapp/internal/models"
)

// Client manages a resilient WebSocket connection with exponential backoff.
type Client struct {
	baseURL   string
	token     string
	onEvent   func(models.WSEvent)
	onStatus  func(bool) // online / offline state
	conn      *websocket.Conn
	mu        sync.Mutex
	sendCh    chan []byte
	closed    bool
	closeOnce sync.Once
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewClient creates a WebSocket client with auto-reconnect.
func NewClient(baseURL, token string, onEvent func(models.WSEvent), onStatus func(bool)) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		baseURL:  baseURL,
		token:    token,
		onEvent:  onEvent,
		onStatus: onStatus,
		sendCh:   make(chan []byte, 128),
		ctx:      ctx,
		cancel:   cancel,
	}
	go c.runLoop()
	return c
}

// UpdateToken updates the auth token used for reconnection and closes any active connection so it reconnects with the new token immediately.
func (c *Client) UpdateToken(token string) {
	c.mu.Lock()
	c.token = token
	conn := c.conn
	c.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

func (c *Client) runLoop() {
	backoff := 1 * time.Second
	const maxBackoff = 30 * time.Second

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.mu.Lock()
		token := c.token
		c.mu.Unlock()

		if token == "" {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		wsURL := strings.Replace(c.baseURL, "http://", "ws://", 1)
		wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
		wsURL = fmt.Sprintf("%s/ws?token=%s", strings.TrimRight(wsURL, "/"), url.QueryEscape(token))

		dialer := websocket.DefaultDialer
		conn, _, err := dialer.DialContext(c.ctx, wsURL, nil)
		if err != nil {
			if c.onStatus != nil {
				c.onStatus(false)
			}
			select {
			case <-time.After(backoff):
				backoff = min(backoff*2, maxBackoff)
			case <-c.ctx.Done():
				return
			}
			continue
		}

		// Connected successfully! Reset backoff
		backoff = 1 * time.Second
		c.mu.Lock()
		c.conn = conn
		c.mu.Unlock()

		if c.onStatus != nil {
			c.onStatus(true)
		}

		// Run read and write pumps
		done := make(chan struct{})
		go c.readPump(conn, done)
		c.writePump(conn, done)

		c.mu.Lock()
		c.conn = nil
		c.mu.Unlock()

		if c.onStatus != nil {
			c.onStatus(false)
		}
	}
}

func (c *Client) readPump(conn *websocket.Conn, done chan struct{}) {
	defer close(done)
	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var evt models.WSEvent
		if err := json.Unmarshal(msgBytes, &evt); err == nil {
			if c.onEvent != nil {
				c.onEvent(evt)
			}
		}
	}
}

func (c *Client) writePump(conn *websocket.Conn, done chan struct{}) {
	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-c.ctx.Done():
			_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		case msg := <-c.sendCh:
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			// Ping heartbeat
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Send dispatches an event to the server.
func (c *Client) Send(event models.WSEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	select {
	case c.sendCh <- data:
		return nil
	default:
		return fmt.Errorf("client send buffer full")
	}
}

// Close terminates the connection and background loops.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		c.cancel()
		c.mu.Lock()
		if c.conn != nil {
			_ = c.conn.Close()
		}
		c.mu.Unlock()
	})
}
