// Package middleware contains HTTP middleware for the API server.
package middleware

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"todoapp/internal/models"
	"todoapp/internal/server/auth"
	"todoapp/internal/server/httpx"
)

type ctxKey int

const (
	userKey ctxKey = iota
	tokenKey
)

// Authenticator resolves a bearer token to a user.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (models.User, error)
}

// UserFrom returns the authenticated user stored by RequireAuth.
func UserFrom(ctx context.Context) (models.User, bool) {
	u, ok := ctx.Value(userKey).(models.User)
	return u, ok
}

// TokenFrom returns the raw bearer token stored by RequireAuth. It lives only
// in memory for the duration of the request (used by logout).
func TokenFrom(ctx context.Context) string {
	t, _ := ctx.Value(tokenKey).(string)
	return t
}

// BearerToken extracts the token from "Authorization: Bearer <token>".
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// RequireAuth rejects requests without a valid session.
func RequireAuth(a Authenticator, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := BearerToken(r)
			if token == "" {
				w.Header().Set("WWW-Authenticate", `Bearer`)
				httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "missing bearer token")
				return
			}
			u, err := a.Authenticate(r.Context(), token)
			if errors.Is(err, auth.ErrUnauthenticated) {
				w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
				httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "invalid or expired session")
				return
			}
			if err != nil {
				log.ErrorContext(r.Context(), "authenticate", "err", err, "request_id", chimw.GetReqID(r.Context()))
				httpx.WriteError(w, http.StatusInternalServerError, models.CodeInternal, "internal error")
				return
			}
			ctx := context.WithValue(r.Context(), userKey, u)
			ctx = context.WithValue(ctx, tokenKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// MaxBody caps the request body size. Reads beyond n fail with
// *http.MaxBytesError, which httpx.DecodeJSON maps to 413.
func MaxBody(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > n {
				httpx.WriteError(w, http.StatusRequestEntityTooLarge, models.CodeTooLarge, "request body too large")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

// SecurityHeaders sets conservative headers for a JSON API.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// Logger logs one line per request. It deliberately logs only the path
// (no query string, no headers, no body) so tokens and passwords never end
// up in logs.
func Logger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			level := slog.LevelInfo
			if status >= 500 {
				level = slog.LevelError
			}
			log.Log(r.Context(), level, "http request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"bytes", ww.BytesWritten(),
				"duration", time.Since(start),
				"ip", ClientIP(r),
				"request_id", chimw.GetReqID(r.Context()),
			)
		})
	}
}

// Recoverer turns panics into a 500 JSON response and logs the stack.
func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if rec == http.ErrAbortHandler {
						panic(rec)
					}
					log.ErrorContext(r.Context(), "panic", "panic", rec, "stack", string(debug.Stack()),
						"request_id", chimw.GetReqID(r.Context()))
					httpx.WriteError(w, http.StatusInternalServerError, models.CodeInternal, "internal error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
