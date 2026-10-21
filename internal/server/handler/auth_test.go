package handler_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"todoapp/internal/models"
	"todoapp/internal/server/auth"
	"todoapp/internal/server/handler"
	"todoapp/internal/server/store/memstore"
	"todoapp/internal/server/ws"
)

type testServer struct {
	*httptest.Server
	t *testing.T
}

func newTestServer(t *testing.T, rate handler.RateLimit) *testServer {
	t.Helper()
	st := memstore.New(nil)
	svc, err := auth.NewService(st, auth.Options{
		SessionTTL:    time.Hour,
		TouchInterval: time.Minute,
		Argon2:        auth.Argon2Params{Memory: 1024, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32},
	})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := ws.NewHub(logger, st.ListRoomMembers)
	h := handler.NewRouter(handler.Deps{
		Auth:         svc,
		Todos:        st,
		Journal:      st,
		Chat:         st,
		UserLookup:   st,
		Hub:          hub,
		Health:       st,
		Logger:       logger,
		MaxBodyBytes: 1024,
		AuthRate:     rate,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &testServer{Server: srv, t: t}
}

var generousRate = handler.RateLimit{PerMinute: 1000, Burst: 1000}

func (s *testServer) do(method, path, token string, body any) (*http.Response, []byte) {
	s.t.Helper()
	var r io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		r = strings.NewReader(b)
	default:
		buf, err := json.Marshal(b)
		if err != nil {
			s.t.Fatal(err)
		}
		r = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, s.URL+path, r)
	if err != nil {
		s.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := s.Client().Do(req)
	if err != nil {
		s.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func decode[T any](t *testing.T, data []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("decode %s: %v", data, err)
	}
	return v
}

func expectStatus(t *testing.T, resp *http.Response, body []byte, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("%s %s: status = %d; want %d; body = %s",
			resp.Request.Method, resp.Request.URL.Path, resp.StatusCode, want, body)
	}
}

func TestAuthFlow(t *testing.T) {
	s := newTestServer(t, generousRate)

	resp, body := s.do("POST", "/api/v1/auth/register", "",
		models.RegisterRequest{Username: "alice", Password: "password123", Timezone: "Asia/Jakarta"})
	expectStatus(t, resp, body, http.StatusCreated)
	reg := decode[models.UserResponse](t, body)
	if reg.User.Username != "alice" || reg.User.Timezone != "Asia/Jakarta" {
		t.Fatalf("register user = %+v", reg.User)
	}
	if strings.Contains(string(body), "password") {
		t.Fatal("register response leaks password data")
	}

	resp, body = s.do("POST", "/api/v1/auth/register", "",
		models.RegisterRequest{Username: "alice", Password: "password123"})
	expectStatus(t, resp, body, http.StatusConflict)

	resp, body = s.do("POST", "/api/v1/auth/login", "",
		models.LoginRequest{Username: "alice", Password: "nope-nope"})
	expectStatus(t, resp, body, http.StatusUnauthorized)
	if e := decode[models.ErrorResponse](t, body); e.Error.Code != models.CodeInvalidCredentials {
		t.Fatalf("code = %q", e.Error.Code)
	}

	resp, body = s.do("POST", "/api/v1/auth/login", "",
		models.LoginRequest{Username: "alice", Password: "password123"})
	expectStatus(t, resp, body, http.StatusOK)
	login := decode[models.LoginResponse](t, body)
	if login.Token == "" || login.ExpiresAt.IsZero() {
		t.Fatalf("login response = %+v", login)
	}

	resp, body = s.do("GET", "/api/v1/me", login.Token, nil)
	expectStatus(t, resp, body, http.StatusOK)
	if me := decode[models.UserResponse](t, body); me.User.ID != reg.User.ID {
		t.Fatalf("me = %+v", me.User)
	}

	resp, body = s.do("POST", "/api/v1/auth/logout", login.Token, nil)
	expectStatus(t, resp, body, http.StatusNoContent)

	resp, body = s.do("GET", "/api/v1/me", login.Token, nil)
	expectStatus(t, resp, body, http.StatusUnauthorized)
}

func TestProtectedRoutesRequireAuth(t *testing.T) {
	s := newTestServer(t, generousRate)
	for _, c := range []struct{ method, path, token string }{
		{"GET", "/api/v1/me", ""},
		{"GET", "/api/v1/me", "not-a-real-token"},
		{"POST", "/api/v1/auth/logout", ""},
	} {
		resp, body := s.do(c.method, c.path, c.token, nil)
		expectStatus(t, resp, body, http.StatusUnauthorized)
		if e := decode[models.ErrorResponse](t, body); e.Error.Code != models.CodeUnauthenticated {
			t.Errorf("%s %s code = %q", c.method, c.path, e.Error.Code)
		}
	}
}

func TestBadRequests(t *testing.T) {
	s := newTestServer(t, generousRate)
	cases := []struct {
		name   string
		body   any
		status int
		code   string
	}{
		{"invalid json", `{"username":`, http.StatusBadRequest, models.CodeBadRequest},
		{"empty body", ``, http.StatusBadRequest, models.CodeBadRequest},
		{"unknown field", `{"username":"bob","password":"password123","admin":true}`, http.StatusBadRequest, models.CodeBadRequest},
		{"wrong type", `{"username":1,"password":"password123"}`, http.StatusBadRequest, models.CodeBadRequest},
		{"two objects", `{"username":"bob","password":"password123"}{}`, http.StatusBadRequest, models.CodeBadRequest},
		{"validation", models.RegisterRequest{Username: "b", Password: "password123"}, http.StatusBadRequest, models.CodeValidation},
		{"too large", `{"username":"bob","password":"` + strings.Repeat("a", 2048) + `"}`, http.StatusRequestEntityTooLarge, models.CodeTooLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := c.body
			if s, ok := body.(string); ok && s == "" {
				body = " " // force a body so Content-Type is set
			}
			resp, data := s.do("POST", "/api/v1/auth/register", "", body)
			expectStatus(t, resp, data, c.status)
			if e := decode[models.ErrorResponse](t, data); e.Error.Code != c.code {
				t.Fatalf("code = %q; want %q (%s)", e.Error.Code, c.code, e.Error.Message)
			}
		})
	}
}

func TestValidationErrorHasField(t *testing.T) {
	s := newTestServer(t, generousRate)
	resp, body := s.do("POST", "/api/v1/auth/register", "",
		models.RegisterRequest{Username: "bob", Password: "short"})
	expectStatus(t, resp, body, http.StatusBadRequest)
	if e := decode[models.ErrorResponse](t, body); e.Error.Field != "password" {
		t.Fatalf("field = %q; want password", e.Error.Field)
	}
}

func TestLoginRateLimitPerIP(t *testing.T) {
	s := newTestServer(t, handler.RateLimit{PerMinute: 1, Burst: 3})
	for i := 0; i < 3; i++ {
		// Different usernames so only the per-IP bucket is exercised.
		resp, body := s.do("POST", "/api/v1/auth/login", "",
			models.LoginRequest{Username: "user" + string(rune('a'+i)) + "x", Password: "password123"})
		expectStatus(t, resp, body, http.StatusUnauthorized)
	}
	resp, body := s.do("POST", "/api/v1/auth/login", "",
		models.LoginRequest{Username: "another", Password: "password123"})
	expectStatus(t, resp, body, http.StatusTooManyRequests)
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After header")
	}
}

func TestHealthAndNotFound(t *testing.T) {
	s := newTestServer(t, generousRate)
	resp, body := s.do("GET", "/healthz", "", nil)
	expectStatus(t, resp, body, http.StatusOK)
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("security headers missing")
	}
	resp, body = s.do("GET", "/api/v1/nope", "", nil)
	expectStatus(t, resp, body, http.StatusNotFound)
}
