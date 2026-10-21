package ws_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"todoapp/internal/client/ws"
	"todoapp/internal/models"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func TestWSClient_ConnectAndReceive(t *testing.T) {
	var receivedEvent models.WSEvent
	var eventMu sync.Mutex
	eventReceived := make(chan struct{}, 1)

	statusCh := make(chan bool, 10)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token != "valid-test-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Send an event from server to client
		evt := models.WSEvent{
			Type:    "message",
			Payload: map[string]any{"id": float64(100), "body": "hello test"},
		}
		data, _ := json.Marshal(evt)
		_ = conn.WriteMessage(websocket.TextMessage, data)

		// Read messages from client until disconnected
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}))
	defer server.Close()

	client := ws.NewClient(
		server.URL,
		"valid-test-token",
		func(evt models.WSEvent) {
			eventMu.Lock()
			receivedEvent = evt
			eventMu.Unlock()
			select {
			case eventReceived <- struct{}{}:
			default:
			}
		},
		func(online bool) {
			statusCh <- online
		},
	)
	defer client.Close()

	// Wait for online status
	select {
	case online := <-statusCh:
		if !online {
			t.Fatalf("expected initial status to be true (online)")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for online status")
	}

	// Wait for event received
	select {
	case <-eventReceived:
		eventMu.Lock()
		defer eventMu.Unlock()
		if receivedEvent.Type != "message" {
			t.Errorf("expected type 'message', got %s", receivedEvent.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for WS event")
	}

	// Test sending
	err := client.Send(models.WSEvent{Type: "ping"})
	if err != nil {
		t.Fatalf("failed to send event: %v", err)
	}

	// Test close
	client.Close()
	select {
	case online := <-statusCh:
		if online {
			t.Errorf("expected status false after close")
		}
	case <-time.After(1 * time.Second):
		// offline notification might already have arrived or triggered
	}
}

func TestWSClient_ReconnectionWithUpdatedToken(t *testing.T) {
	authFailed := make(chan struct{}, 1)
	authSuccess := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "new-token" {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			select {
			case authSuccess <- struct{}{}:
			default:
			}
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					break
				}
			}
			return
		}

		select {
		case authFailed <- struct{}{}:
		default:
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	client := ws.NewClient(
		strings.Replace(server.URL, "http://", "ws://", 1),
		"old-bad-token",
		nil,
		nil,
	)
	defer client.Close()

	select {
	case <-authFailed:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected initial auth failure")
	}

	// Update token
	client.UpdateToken("new-token")

	select {
	case <-authSuccess:
		// Reconnected with new token successfully!
	case <-time.After(4 * time.Second):
		t.Fatalf("timeout waiting for reconnect with updated token")
	}
}
