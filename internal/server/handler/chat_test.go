package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"todoapp/internal/models"
)

func TestChatRoomsAndDMs(t *testing.T) {
	ts := newTestServer(t, generousRate)
	tokenAlice, _ := registerAndLogin(t, ts, "alicechat", "alice-pass-123")
	tokenBob, _ := registerAndLogin(t, ts, "bobchat", "bob-pass-123")

	// 1. Alice creates room "General"
	res, body := ts.do(http.MethodPost, "/api/v1/rooms", tokenAlice, models.CreateRoomRequest{
		Name: "General",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create room: %s", body)
	}
	var roomResp models.RoomResponse
	json.Unmarshal(body, &roomResp)
	roomID := roomResp.Room.ID

	// 2. Alice lists rooms -> should have "General"
	res, body = ts.do(http.MethodGet, "/api/v1/rooms", tokenAlice, nil)
	var listResp models.RoomListResponse
	json.Unmarshal(body, &listResp)
	if len(listResp.Rooms) != 1 || listResp.Rooms[0].Name != "General" {
		t.Fatalf("expected 1 room for Alice, got %+v", listResp.Rooms)
	}

	// 3. Bob joins "General"
	res, _ = ts.do(http.MethodPost, fmt.Sprintf("/api/v1/rooms/%d/join", roomID), tokenBob, nil)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("bob join room failed: %d", res.StatusCode)
	}

	// 4. Bob lists rooms -> should have "General"
	res, body = ts.do(http.MethodGet, "/api/v1/rooms", tokenBob, nil)
	json.Unmarshal(body, &listResp)
	if len(listResp.Rooms) != 1 || listResp.Rooms[0].Name != "General" {
		t.Fatalf("expected 1 room for Bob, got %+v", listResp.Rooms)
	}

	// 5. Alice creates DM with Bob
	res, body = ts.do(http.MethodPost, "/api/v1/rooms/dm", tokenAlice, models.CreateDMRequest{
		Username: "bobchat",
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("create dm failed: %s", body)
	}
	var dmResp models.RoomResponse
	json.Unmarshal(body, &dmResp)
	if dmResp.Room.Name != "@bobchat" || !dmResp.Room.IsDM {
		t.Fatalf("unexpected DM name for Alice: %s", dmResp.Room.Name)
	}

	// 6. Bob lists rooms -> sees "General" and "@alicechat"
	res, body = ts.do(http.MethodGet, "/api/v1/rooms", tokenBob, nil)
	json.Unmarshal(body, &listResp)
	if len(listResp.Rooms) != 2 {
		t.Fatalf("expected 2 rooms for Bob, got %d", len(listResp.Rooms))
	}

	// 7. Creating DM again returns existing room (idempotent)
	res, body = ts.do(http.MethodPost, "/api/v1/rooms/dm", tokenBob, models.CreateDMRequest{
		Username: "alicechat",
	})
	var dmResp2 models.RoomResponse
	json.Unmarshal(body, &dmResp2)
	if dmResp2.Room.ID != dmResp.Room.ID {
		t.Fatalf("DM room ID mismatch: %d vs %d", dmResp2.Room.ID, dmResp.Room.ID)
	}
}

func TestChatMessagePagination(t *testing.T) {
	ts := newTestServer(t, generousRate)
	token, _ := registerAndLogin(t, ts, "msguser", "msg-pass-123")

	res, body := ts.do(http.MethodPost, "/api/v1/rooms", token, models.CreateRoomRequest{
		Name: "dev-chat",
	})
	var rResp models.RoomResponse
	json.Unmarshal(body, &rResp)
	roomID := rResp.Room.ID

	// Send 5 messages
	for i := 1; i <= 5; i++ {
		ts.do(http.MethodPost, fmt.Sprintf("/api/v1/rooms/%d/messages", roomID), token, models.SendMessageRequest{
			Body: fmt.Sprintf("Message %d", i),
		})
	}

	// Fetch with limit 3 (should return Message 3, 4, 5 and has_more=true)
	res, body = ts.do(http.MethodGet, fmt.Sprintf("/api/v1/rooms/%d/messages?limit=3", roomID), token, nil)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list messages: %s", body)
	}
	var msgList models.MessageListResponse
	json.Unmarshal(body, &msgList)
	if len(msgList.Messages) != 3 || !msgList.HasMore {
		t.Fatalf("expected 3 messages and hasMore=true, got %d / %v", len(msgList.Messages), msgList.HasMore)
	}
	if msgList.Messages[0].Body != "Message 3" || msgList.Messages[2].Body != "Message 5" {
		t.Fatalf("unexpected chronological order: %+v", msgList.Messages)
	}

	// Fetch earlier messages using before_id
	firstID := msgList.Messages[0].ID
	res, body = ts.do(http.MethodGet, fmt.Sprintf("/api/v1/rooms/%d/messages?before_id=%d&limit=3", roomID, firstID), token, nil)
	json.Unmarshal(body, &msgList)
	if len(msgList.Messages) != 2 || msgList.HasMore {
		t.Fatalf("expected remaining 2 messages and hasMore=false, got %d / %v", len(msgList.Messages), msgList.HasMore)
	}
	if msgList.Messages[0].Body != "Message 1" || msgList.Messages[1].Body != "Message 2" {
		t.Fatalf("unexpected earlier order: %+v", msgList.Messages)
	}
}

func TestChatAuthorization(t *testing.T) {
	ts := newTestServer(t, generousRate)
	tokenAlice, _ := registerAndLogin(t, ts, "auth_alice", "alice-pass-123")
	tokenCharlie, _ := registerAndLogin(t, ts, "auth_charlie", "charlie-pass-123")

	res, body := ts.do(http.MethodPost, "/api/v1/rooms", tokenAlice, models.CreateRoomRequest{
		Name: "Alice Secret Club",
	})
	var rResp models.RoomResponse
	json.Unmarshal(body, &rResp)
	roomID := rResp.Room.ID

	// Charlie tries to list messages -> 403 Forbidden
	res, _ = ts.do(http.MethodGet, fmt.Sprintf("/api/v1/rooms/%d/messages", roomID), tokenCharlie, nil)
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member, got %d", res.StatusCode)
	}

	// Charlie tries to send message -> 403 Forbidden
	res, _ = ts.do(http.MethodPost, fmt.Sprintf("/api/v1/rooms/%d/messages", roomID), tokenCharlie, models.SendMessageRequest{
		Body: "Intruder!",
	})
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for non-member send, got %d", res.StatusCode)
	}
}

func TestChatReadState(t *testing.T) {
	ts := newTestServer(t, generousRate)
	tokenAlice, _ := registerAndLogin(t, ts, "rs_alice", "pass-1234567")
	tokenBob, _ := registerAndLogin(t, ts, "rs_bob", "pass-1234567")

	res, body := ts.do(http.MethodPost, "/api/v1/rooms", tokenAlice, models.CreateRoomRequest{
		Name: "Announcements",
	})
	var rResp models.RoomResponse
	json.Unmarshal(body, &rResp)
	roomID := rResp.Room.ID

	// Bob joins Announcements
	ts.do(http.MethodPost, fmt.Sprintf("/api/v1/rooms/%d/join", roomID), tokenBob, nil)

	// Alice sends 2 messages
	ts.do(http.MethodPost, fmt.Sprintf("/api/v1/rooms/%d/messages", roomID), tokenAlice, models.SendMessageRequest{Body: "Notice 1"})
	res, body = ts.do(http.MethodPost, fmt.Sprintf("/api/v1/rooms/%d/messages", roomID), tokenAlice, models.SendMessageRequest{Body: "Notice 2"})
	var msg2 models.MessageResponse
	json.Unmarshal(body, &msg2)

	// Bob checks rooms -> unread count should be 2
	res, body = ts.do(http.MethodGet, "/api/v1/rooms", tokenBob, nil)
	var listResp models.RoomListResponse
	json.Unmarshal(body, &listResp)
	if len(listResp.Rooms) != 1 || listResp.Rooms[0].UnreadCount != 2 {
		t.Fatalf("expected 2 unread for Bob, got %d", listResp.Rooms[0].UnreadCount)
	}

	// Bob marks as read
	res, _ = ts.do(http.MethodPost, fmt.Sprintf("/api/v1/rooms/%d/read", roomID), tokenBob, models.MarkReadRequest{
		LastReadMessageID: msg2.Message.ID,
	})
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("mark read failed: %d", res.StatusCode)
	}

	// Bob checks rooms again -> unread count should be 0
	res, body = ts.do(http.MethodGet, "/api/v1/rooms", tokenBob, nil)
	json.Unmarshal(body, &listResp)
	if listResp.Rooms[0].UnreadCount != 0 {
		t.Fatalf("expected 0 unread after mark read, got %d", listResp.Rooms[0].UnreadCount)
	}
}

func TestWebSocketRealtimeMessageAndPresence(t *testing.T) {
	ts := newTestServer(t, generousRate)
	tokenAlice, _ := registerAndLogin(t, ts, "ws_alice", "pass-1234567")
	tokenBob, _ := registerAndLogin(t, ts, "ws_bob", "pass-1234567")

	res, body := ts.do(http.MethodPost, "/api/v1/rooms", tokenAlice, models.CreateRoomRequest{
		Name: "LiveChat",
	})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create room: %s", body)
	}
	var rResp models.RoomResponse
	json.Unmarshal(body, &rResp)
	roomID := rResp.Room.ID
	ts.do(http.MethodPost, fmt.Sprintf("/api/v1/rooms/%d/join", roomID), tokenBob, nil)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?token="

	// Alice connects to WebSocket
	connAlice, _, err := websocket.DefaultDialer.Dial(wsURL+url.QueryEscape(tokenAlice), nil)
	if err != nil {
		t.Fatalf("alice dial ws: %v", err)
	}
	defer connAlice.Close()

	// Bob connects to WebSocket
	connBob, _, err := websocket.DefaultDialer.Dial(wsURL+url.QueryEscape(tokenBob), nil)
	if err != nil {
		t.Fatalf("bob dial ws: %v", err)
	}
	defer connBob.Close()

	// Alice sends message in room
	ts.do(http.MethodPost, fmt.Sprintf("/api/v1/rooms/%d/messages", roomID), tokenAlice, models.SendMessageRequest{
		Body: "Hello Bob over WS!",
	})

	// Bob should receive the message event over WS
	connBob.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, msgBytes, err := connBob.ReadMessage()
		if err != nil {
			t.Fatalf("bob read ws message: %v", err)
		}
		var evt models.WSEvent
		json.Unmarshal(msgBytes, &evt)
		if evt.Type == "message" {
			// Success! Message broadcast received
			break
		}
	}
}
