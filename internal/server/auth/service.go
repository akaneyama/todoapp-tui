// Package auth implements registration, login, logout and session
// authentication on top of a pluggable Store.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"todoapp/internal/models"
	"todoapp/internal/server/store"
)

// Store is the persistence the auth service needs. Implemented by
// *store.Store (MariaDB) and *memstore.Store (tests).
type Store interface {
	CreateUser(ctx context.Context, username, passwordHash, timezone string) (store.User, error)
	GetUserByUsername(ctx context.Context, username string) (store.User, error)
	UpdateUserTimezone(ctx context.Context, userID int64, timezone string) error
	CreateSession(ctx context.Context, s store.Session) error
	GetSessionWithUser(ctx context.Context, tokenHash []byte, now time.Time) (store.Session, store.User, error)
	TouchSession(ctx context.Context, tokenHash []byte, lastUsedAt, expiresAt time.Time) error
	DeleteSession(ctx context.Context, tokenHash []byte) error
	DeleteExpiredSessions(ctx context.Context, now time.Time) (int64, error)
}

// Errors returned by Service. Handlers map these to HTTP status codes.
var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUsernameTaken      = errors.New("username already taken")
	ErrUnauthenticated    = errors.New("unauthenticated")
)

// ValidationError reports invalid user input.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// Input limits.
const (
	MinPasswordLen = 8
	MaxPasswordLen = 128 // bytes; bounds argon2 input and request size
	maxTokenLen    = 128
	maxTimezoneLen = 64
	DefaultTZ      = "UTC"
)

var usernameRE = regexp.MustCompile(`^[a-z][a-z0-9_]{2,31}$`)

// Options configures a Service.
type Options struct {
	SessionTTL    time.Duration
	TouchInterval time.Duration // min time between sliding-expiry updates
	Argon2        Argon2Params
	Now           func() time.Time // defaults to time.Now
	Logger        *slog.Logger     // defaults to slog.Default()
}

// Service implements the authentication use cases.
type Service struct {
	store         Store
	ttl           time.Duration
	touchInterval time.Duration
	argon         Argon2Params
	now           func() time.Time
	log           *slog.Logger
	// dummyHash is verified against when the username does not exist, so
	// response time does not reveal which usernames are registered.
	dummyHash string
}

// NewService builds a Service.
func NewService(st Store, opt Options) (*Service, error) {
	if opt.SessionTTL <= 0 {
		return nil, errors.New("auth: SessionTTL must be positive")
	}
	if opt.Argon2 == (Argon2Params{}) {
		opt.Argon2 = DefaultArgon2Params
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.Logger == nil {
		opt.Logger = slog.Default()
	}
	dummy, err := HashPassword("dummy-password-for-timing", opt.Argon2)
	if err != nil {
		return nil, fmt.Errorf("auth: dummy hash: %w", err)
	}
	return &Service{
		store:         st,
		ttl:           opt.SessionTTL,
		touchInterval: opt.TouchInterval,
		argon:         opt.Argon2,
		now:           func() time.Time { return opt.Now().UTC() },
		log:           opt.Logger,
		dummyHash:     dummy,
	}, nil
}

// NormalizeUsername trims and lower-cases a username.
func NormalizeUsername(u string) string { return strings.ToLower(strings.TrimSpace(u)) }

// ValidateUsername checks an already-normalized username.
func ValidateUsername(u string) error {
	if !usernameRE.MatchString(u) {
		return &ValidationError{Field: "username", Message: "must be 3-32 characters: a-z, 0-9, _ and start with a letter"}
	}
	return nil
}

// ValidatePassword checks password length bounds.
func ValidatePassword(p string) error {
	if len(p) < MinPasswordLen || len(p) > MaxPasswordLen {
		return &ValidationError{Field: "password", Message: fmt.Sprintf("must be %d-%d bytes", MinPasswordLen, MaxPasswordLen)}
	}
	return nil
}

// ValidateTimezone checks that tz is a loadable IANA zone name.
func ValidateTimezone(tz string) error {
	if tz == "" || len(tz) > maxTimezoneLen || tz == "Local" {
		return &ValidationError{Field: "timezone", Message: "must be an IANA timezone name, e.g. Asia/Jakarta"}
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return &ValidationError{Field: "timezone", Message: "unknown timezone " + tz}
	}
	return nil
}

// Register creates a new account.
func (s *Service) Register(ctx context.Context, username, password, timezone string) (models.User, error) {
	username = NormalizeUsername(username)
	if err := ValidateUsername(username); err != nil {
		return models.User{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return models.User{}, err
	}
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = DefaultTZ
	}
	if err := ValidateTimezone(timezone); err != nil {
		return models.User{}, err
	}

	hash, err := HashPassword(password, s.argon)
	if err != nil {
		return models.User{}, fmt.Errorf("register: %w", err)
	}
	u, err := s.store.CreateUser(ctx, username, hash, timezone)
	if errors.Is(err, store.ErrConflict) {
		return models.User{}, ErrUsernameTaken
	}
	if err != nil {
		return models.User{}, fmt.Errorf("register: %w", err)
	}
	return u.Public(), nil
}

// LoginResult is returned by Login.
type LoginResult struct {
	Token     string
	ExpiresAt time.Time
	User      models.User
}

// Login verifies credentials and creates a session. If timezone is non-empty
// and valid it replaces the stored timezone.
func (s *Service) Login(ctx context.Context, username, password, timezone string) (LoginResult, error) {
	username = NormalizeUsername(username)
	timezone = strings.TrimSpace(timezone)
	if timezone != "" {
		if err := ValidateTimezone(timezone); err != nil {
			return LoginResult{}, err
		}
	}
	if len(password) > MaxPasswordLen || ValidateUsername(username) != nil {
		// Cannot possibly match; still burn the same CPU time.
		_, _ = VerifyPassword(password, s.dummyHash)
		return LoginResult{}, ErrInvalidCredentials
	}

	u, err := s.store.GetUserByUsername(ctx, username)
	if errors.Is(err, store.ErrNotFound) {
		_, _ = VerifyPassword(password, s.dummyHash)
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, fmt.Errorf("login: %w", err)
	}
	ok, err := VerifyPassword(password, u.PasswordHash)
	if err != nil {
		return LoginResult{}, fmt.Errorf("login: verify password for user %d: %w", u.ID, err)
	}
	if !ok {
		return LoginResult{}, ErrInvalidCredentials
	}

	if timezone != "" && timezone != u.Timezone {
		if err := s.store.UpdateUserTimezone(ctx, u.ID, timezone); err != nil {
			return LoginResult{}, fmt.Errorf("login: %w", err)
		}
		u.Timezone = timezone
	}

	token, err := NewToken()
	if err != nil {
		return LoginResult{}, fmt.Errorf("login: %w", err)
	}
	now := s.now()
	sess := store.Session{
		TokenHash:  HashToken(token),
		UserID:     u.ID,
		ExpiresAt:  now.Add(s.ttl),
		CreatedAt:  now,
		LastUsedAt: now,
	}
	if err := s.store.CreateSession(ctx, sess); err != nil {
		return LoginResult{}, fmt.Errorf("login: %w", err)
	}
	return LoginResult{Token: token, ExpiresAt: sess.ExpiresAt, User: u.Public()}, nil
}

// Logout deletes the session for token. It is idempotent.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" || len(token) > maxTokenLen {
		return nil
	}
	if err := s.store.DeleteSession(ctx, HashToken(token)); err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	return nil
}

// Authenticate resolves a bearer token to its user, extending the session
// expiry (sliding) at most once per TouchInterval.
func (s *Service) Authenticate(ctx context.Context, token string) (models.User, error) {
	if token == "" || len(token) > maxTokenLen {
		return models.User{}, ErrUnauthenticated
	}
	hash := HashToken(token)
	now := s.now()
	sess, u, err := s.store.GetSessionWithUser(ctx, hash, now)
	if errors.Is(err, store.ErrNotFound) {
		return models.User{}, ErrUnauthenticated
	}
	if err != nil {
		return models.User{}, fmt.Errorf("authenticate: %w", err)
	}
	if now.Sub(sess.LastUsedAt) >= s.touchInterval {
		if err := s.store.TouchSession(ctx, hash, now, now.Add(s.ttl)); err != nil {
			// Not fatal: the session is still valid until its old expiry.
			s.log.WarnContext(ctx, "failed to extend session", "user_id", u.ID, "err", err)
		}
	}
	return u.Public(), nil
}

// CleanupExpired deletes expired sessions.
func (s *Service) CleanupExpired(ctx context.Context) (int64, error) {
	n, err := s.store.DeleteExpiredSessions(ctx, s.now())
	if err != nil {
		return 0, fmt.Errorf("cleanup sessions: %w", err)
	}
	return n, nil
}

// RunJanitor periodically deletes expired sessions until ctx is cancelled.
func (s *Service) RunJanitor(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := s.CleanupExpired(ctx)
			if err != nil {
				if ctx.Err() == nil {
					s.log.ErrorContext(ctx, "session janitor failed", "err", err)
				}
				continue
			}
			if n > 0 {
				s.log.InfoContext(ctx, "expired sessions removed", "count", n)
			}
		}
	}
}
