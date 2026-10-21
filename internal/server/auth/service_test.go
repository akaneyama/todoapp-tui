package auth_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"todoapp/internal/server/auth"
	"todoapp/internal/server/store/memstore"
)

var fastParams = auth.Argon2Params{Memory: 1024, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

// clock is a controllable time source.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time          { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Advance(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

func newService(t *testing.T) (*auth.Service, *memstore.Store, *clock) {
	t.Helper()
	clk := &clock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	st := memstore.New(clk.Now)
	svc, err := auth.NewService(st, auth.Options{
		SessionTTL:    30 * 24 * time.Hour,
		TouchInterval: time.Hour,
		Argon2:        fastParams,
		Now:           clk.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, st, clk
}

func TestRegisterValidation(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	cases := []struct {
		name, user, pass, tz, field string
	}{
		{"short username", "ab", "password123", "", "username"},
		{"bad chars", "bob!", "password123", "", "username"},
		{"starts with digit", "1bob", "password123", "", "username"},
		{"too long", "a234567890123456789012345678901234", "password123", "", "username"},
		{"short password", "bob", "short", "", "password"},
		{"long password", "bob", string(make([]byte, 129)), "", "password"},
		{"bad timezone", "bob", "password123", "Mars/Olympus", "timezone"},
		{"Local timezone", "bob", "password123", "Local", "timezone"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.Register(ctx, c.user, c.pass, c.tz)
			var verr *auth.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("err = %v; want ValidationError", err)
			}
			if verr.Field != c.field {
				t.Fatalf("field = %q; want %q", verr.Field, c.field)
			}
		})
	}
}

func TestRegisterNormalizesAndDefaults(t *testing.T) {
	svc, _, _ := newService(t)
	u, err := svc.Register(context.Background(), "  Alice_01 ", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != "alice_01" {
		t.Errorf("username = %q; want alice_01", u.Username)
	}
	if u.Timezone != "UTC" {
		t.Errorf("timezone = %q; want UTC", u.Timezone)
	}
	if u.ID == 0 {
		t.Error("id not set")
	}
}

func TestRegisterDuplicate(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, "alice", "password123", "Asia/Jakarta"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Register(ctx, "ALICE", "password456", ""); !errors.Is(err, auth.ErrUsernameTaken) {
		t.Fatalf("err = %v; want ErrUsernameTaken", err)
	}
}

func TestLoginAndAuthenticate(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	reg, err := svc.Register(ctx, "alice", "password123", "UTC")
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct{ user, pass string }{
		{"alice", "wrongpass1"},
		{"nobody", "password123"},
		{"x", "password123"}, // invalid format: still invalid credentials
	} {
		if _, err := svc.Login(ctx, c.user, c.pass, ""); !errors.Is(err, auth.ErrInvalidCredentials) {
			t.Errorf("Login(%q,%q) err = %v; want ErrInvalidCredentials", c.user, c.pass, err)
		}
	}

	res, err := svc.Login(ctx, "Alice", "password123", "Asia/Jakarta")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if res.Token == "" || res.User.ID != reg.ID {
		t.Fatalf("unexpected login result %+v", res)
	}
	if res.User.Timezone != "Asia/Jakarta" {
		t.Errorf("timezone not updated on login: %q", res.User.Timezone)
	}

	u, err := svc.Authenticate(ctx, res.Token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if u.ID != reg.ID || u.Timezone != "Asia/Jakarta" {
		t.Fatalf("Authenticate user = %+v", u)
	}

	for _, bad := range []string{"", "garbage", res.Token + "x"} {
		if _, err := svc.Authenticate(ctx, bad); !errors.Is(err, auth.ErrUnauthenticated) {
			t.Errorf("Authenticate(%q) err = %v; want ErrUnauthenticated", bad, err)
		}
	}
}

func TestLoginRejectsInvalidTimezone(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "alice", "password123", "")
	var verr *auth.ValidationError
	if _, err := svc.Login(ctx, "alice", "password123", "Not/AZone"); !errors.As(err, &verr) {
		t.Fatalf("err = %v; want ValidationError", err)
	}
}

func TestSessionExpiryAndSlidingRefresh(t *testing.T) {
	svc, _, clk := newService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "alice", "password123", "")
	res, err := svc.Login(ctx, "alice", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	if want := clk.Now().Add(30 * 24 * time.Hour); !res.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v; want %v", res.ExpiresAt, want)
	}

	// Use the session every 20 days: each use slides the expiry forward,
	// so it stays valid well past the original 30 days.
	for i := 0; i < 3; i++ {
		clk.Advance(20 * 24 * time.Hour)
		if _, err := svc.Authenticate(ctx, res.Token); err != nil {
			t.Fatalf("day %d: session should still be valid: %v", (i+1)*20, err)
		}
	}

	// Idle for longer than the TTL: expired.
	clk.Advance(30*24*time.Hour + time.Second)
	if _, err := svc.Authenticate(ctx, res.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("err = %v; want ErrUnauthenticated after idle > TTL", err)
	}
}

func TestSlidingRefreshThrottled(t *testing.T) {
	svc, _, clk := newService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "alice", "password123", "")
	res, _ := svc.Login(ctx, "alice", "password123", "")
	originalExpiry := res.ExpiresAt

	// Used 30 minutes later: within TouchInterval, expiry is not extended.
	clk.Advance(30 * time.Minute)
	_, _ = svc.Authenticate(ctx, res.Token)

	// Jump to just past the original expiry; if the 30-minute use had
	// extended the session it would still be valid.
	clk.Advance(originalExpiry.Sub(clk.Now()) + time.Second)
	if _, err := svc.Authenticate(ctx, res.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("expiry was extended within TouchInterval; err = %v", err)
	}
}

func TestLogout(t *testing.T) {
	svc, st, _ := newService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "alice", "password123", "")
	a, _ := svc.Login(ctx, "alice", "password123", "")
	b, _ := svc.Login(ctx, "alice", "password123", "")

	if err := svc.Logout(ctx, a.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, a.Token); !errors.Is(err, auth.ErrUnauthenticated) {
		t.Fatalf("token a still valid after logout: %v", err)
	}
	if _, err := svc.Authenticate(ctx, b.Token); err != nil {
		t.Fatalf("logout of a must not affect session b: %v", err)
	}
	if err := svc.Logout(ctx, a.Token); err != nil {
		t.Fatalf("logout must be idempotent: %v", err)
	}
	if st.SessionCount() != 1 {
		t.Fatalf("sessions = %d; want 1", st.SessionCount())
	}
}

func TestCleanupExpired(t *testing.T) {
	svc, st, clk := newService(t)
	ctx := context.Background()
	_, _ = svc.Register(ctx, "alice", "password123", "")
	_, _ = svc.Login(ctx, "alice", "password123", "")
	clk.Advance(31 * 24 * time.Hour)
	_, _ = svc.Login(ctx, "alice", "password123", "")

	n, err := svc.CleanupExpired(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || st.SessionCount() != 1 {
		t.Fatalf("removed %d, remaining %d; want 1 and 1", n, st.SessionCount())
	}
}
