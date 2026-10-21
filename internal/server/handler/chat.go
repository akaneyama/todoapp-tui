package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"

	"todoapp/internal/models"
	"todoapp/internal/server/auth"
	"todoapp/internal/server/httpx"
	"todoapp/internal/server/middleware"
	"todoapp/internal/server/store"
	wsHub "todoapp/internal/server/ws"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow local clients and reverse-proxied clients
		return true
	},
}

func (a *api) handleListRooms(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	rooms, err := a.chat.ListUserRooms(r.Context(), u.ID)
	if err != nil {
		a.internalError(w, r, "list user rooms", err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, models.RoomListResponse{Rooms: rooms})
}

func (a *api) handleCreateRoom(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	var req models.CreateRoomRequest
	if derr := httpx.DecodeJSON(r, &req); derr != nil {
		derr.Write(w)
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		httpx.WriteFieldError(w, "name", "room name cannot be empty")
		return
	}
	if len(name) > 64 {
		httpx.WriteFieldError(w, "name", "room name cannot exceed 64 characters")
		return
	}

	room, err := a.chat.CreateRoom(r.Context(), name, false, nil)
	if err != nil {
		a.internalError(w, r, "create room", err)
		return
	}

	// Add creator as member
	if err := a.chat.AddRoomMember(r.Context(), room.ID, u.ID); err != nil {
		a.internalError(w, r, "add room creator", err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, models.RoomResponse{
		Room: models.Room{
			ID:        room.ID,
			Name:      room.Name,
			IsDM:      false,
			CreatedAt: room.CreatedAt,
		},
	})
}

func (a *api) handleCreateDM(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	var req models.CreateDMRequest
	if derr := httpx.DecodeJSON(r, &req); derr != nil {
		derr.Write(w)
		return
	}

	targetUsername := auth.NormalizeUsername(req.Username)
	if targetUsername == "" {
		httpx.WriteFieldError(w, "username", "target username cannot be empty")
		return
	}
	if strings.EqualFold(targetUsername, u.Username) {
		httpx.WriteFieldError(w, "username", "cannot create a DM with yourself")
		return
	}

	targetUser, err := a.userLookup.GetUserByUsername(r.Context(), targetUsername)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, models.CodeNotFound, "user not found")
			return
		}
		a.internalError(w, r, "get target user", err)
		return
	}

	room, err := a.chat.GetOrCreateDMRoom(r.Context(), u.ID, targetUser.ID)
	if err != nil {
		a.internalError(w, r, "get or create dm", err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, models.RoomResponse{
		Room: models.Room{
			ID:        room.ID,
			Name:      "@" + targetUser.Username,
			IsDM:      true,
			CreatedAt: room.CreatedAt,
		},
	})
}

func (a *api) handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	roomID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, models.CodeBadRequest, "invalid room id")
		return
	}

	room, err := a.chat.GetRoom(r.Context(), roomID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, models.CodeNotFound, "room not found")
			return
		}
		a.internalError(w, r, "get room for join", err)
		return
	}

	if room.IsDM {
		httpx.WriteError(w, http.StatusForbidden, models.CodeForbidden, "cannot join a direct message room")
		return
	}

	if err := a.chat.AddRoomMember(r.Context(), roomID, u.ID); err != nil {
		a.internalError(w, r, "join room", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) handleListMessages(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	roomID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, models.CodeBadRequest, "invalid room id")
		return
	}

	// Verify membership
	isMember, err := a.chat.IsRoomMember(r.Context(), roomID, u.ID)
	if err != nil {
		a.internalError(w, r, "check membership", err)
		return
	}
	if !isMember {
		httpx.WriteError(w, http.StatusForbidden, models.CodeForbidden, "you are not a member of this room")
		return
	}

	q := r.URL.Query()
	beforeID, _ := strconv.ParseInt(q.Get("before_id"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}

	msgs, hasMore, err := a.chat.ListMessages(r.Context(), roomID, beforeID, limit)
	if err != nil {
		a.internalError(w, r, "list messages", err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, models.MessageListResponse{
		Messages: msgs,
		HasMore:  hasMore,
	})
}

func (a *api) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	roomID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, models.CodeBadRequest, "invalid room id")
		return
	}

	isMember, err := a.chat.IsRoomMember(r.Context(), roomID, u.ID)
	if err != nil {
		a.internalError(w, r, "check membership", err)
		return
	}
	if !isMember {
		httpx.WriteError(w, http.StatusForbidden, models.CodeForbidden, "you are not a member of this room")
		return
	}

	var req models.SendMessageRequest
	if derr := httpx.DecodeJSON(r, &req); derr != nil {
		derr.Write(w)
		return
	}

	body := strings.TrimSpace(req.Body)
	if body == "" {
		httpx.WriteFieldError(w, "body", "message body cannot be empty")
		return
	}
	if len(body) > 10000 {
		httpx.WriteFieldError(w, "body", "message body exceeds 10,000 characters")
		return
	}

	msg, err := a.chat.CreateMessage(r.Context(), roomID, u.ID, body)
	if err != nil {
		a.internalError(w, r, "create message", err)
		return
	}

	// Update sender's read_state
	_ = a.chat.UpdateReadState(r.Context(), u.ID, roomID, msg.ID)

	// Broadcast via WebSocket
	if a.hub != nil {
		_ = a.hub.BroadcastToRoom(r.Context(), roomID, models.WSEvent{
			Type:    "message",
			Payload: msg,
		})
	}

	httpx.WriteJSON(w, http.StatusCreated, models.MessageResponse{Message: msg})
}

func (a *api) handleMarkRead(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}

	roomID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, models.CodeBadRequest, "invalid room id")
		return
	}

	var req models.MarkReadRequest
	if derr := httpx.DecodeJSON(r, &req); derr != nil {
		derr.Write(w)
		return
	}

	if err := a.chat.UpdateReadState(r.Context(), u.ID, roomID, req.LastReadMessageID); err != nil {
		a.internalError(w, r, "update read state", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *api) handleWS(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}
	if token == "" {
		http.Error(w, "missing auth token", http.StatusUnauthorized)
		return
	}

	u, err := a.auth.Authenticate(r.Context(), token)
	if err != nil {
		http.Error(w, "invalid or expired token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		a.log.Warn("websocket upgrade failed", "err", err)
		return
	}

	client := &wsHub.Client{
		Hub:      a.hub,
		UserID:   u.ID,
		Username: u.Username,
		Conn:     conn,
		Send:     make(chan []byte, 256),
	}

	a.hub.Register(client)
	go client.WritePump()
	go client.ReadPump()
}
