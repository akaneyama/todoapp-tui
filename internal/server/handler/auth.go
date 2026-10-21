package handler

import (
	"errors"
	"net/http"

	"todoapp/internal/models"
	"todoapp/internal/server/auth"
	"todoapp/internal/server/httpx"
	"todoapp/internal/server/middleware"
)

func (a *api) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if derr := httpx.DecodeJSON(r, &req); derr != nil {
		derr.Write(w)
		return
	}
	u, err := a.auth.Register(r.Context(), req.Username, req.Password, req.Timezone)
	if err != nil {
		a.writeAuthError(w, r, "register", err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, models.UserResponse{User: u})
}

func (a *api) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if derr := httpx.DecodeJSON(r, &req); derr != nil {
		derr.Write(w)
		return
	}
	// Per-username bucket in addition to the per-IP one, so a distributed
	// brute force against a single account is throttled too.
	if ok, wait := a.usernameLimit.Allow(auth.NormalizeUsername(req.Username)); !ok {
		middleware.WriteRateLimited(w, wait)
		return
	}
	res, err := a.auth.Login(r.Context(), req.Username, req.Password, req.Timezone)
	if err != nil {
		a.writeAuthError(w, r, "login", err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, models.LoginResponse{
		Token:     res.Token,
		ExpiresAt: res.ExpiresAt,
		User:      res.User,
	})
}

func (a *api) handleLogout(w http.ResponseWriter, r *http.Request) {
	if err := a.auth.Logout(r.Context(), middleware.TokenFrom(r.Context())); err != nil {
		a.internalError(w, r, "logout", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) handleMe(w http.ResponseWriter, r *http.Request) {
	u, ok := middleware.UserFrom(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeUnauthenticated, "unauthenticated")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, models.UserResponse{User: u})
}

func (a *api) writeAuthError(w http.ResponseWriter, r *http.Request, op string, err error) {
	var verr *auth.ValidationError
	switch {
	case errors.As(err, &verr):
		httpx.WriteFieldError(w, verr.Field, verr.Message)
	case errors.Is(err, auth.ErrUsernameTaken):
		httpx.WriteError(w, http.StatusConflict, models.CodeConflict, "username already taken")
	case errors.Is(err, auth.ErrInvalidCredentials):
		httpx.WriteError(w, http.StatusUnauthorized, models.CodeInvalidCredentials, "invalid username or password")
	default:
		a.internalError(w, r, op, err)
	}
}
