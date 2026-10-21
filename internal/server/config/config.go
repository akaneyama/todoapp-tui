// Package config loads server configuration from environment variables,
// optionally seeded from a .env file.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config is the full server configuration.
type Config struct {
	// Addr is the listen address, e.g. ":8080".
	Addr string
	// DatabaseDSN is a go-sql-driver/mysql DSN. parseTime, loc and
	// time_zone are forced by the db package regardless of what is set here.
	DatabaseDSN string
	// AutoMigrate runs pending migrations on startup.
	AutoMigrate bool

	SessionTTL           time.Duration
	SessionTouchInterval time.Duration

	// TrustProxy makes the server honour X-Forwarded-For / X-Real-IP. Only
	// enable this when the server sits behind a reverse proxy you control
	// (Caddy/nginx), otherwise clients can spoof their IP and bypass rate
	// limits.
	TrustProxy bool

	// LoginRatePerMinute and LoginBurst control the per-IP and per-username
	// token buckets on /auth/login and /auth/register.
	LoginRatePerMinute float64
	LoginBurst         int

	// MaxBodyBytes caps every request body.
	MaxBodyBytes int64

	LogLevel slog.Level
}

// Load reads configuration. If envFile is non-empty and exists it is loaded
// first; real environment variables always take precedence over the file.
func Load(envFile string) (Config, error) {
	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Config{}, fmt.Errorf("load %s: %w", envFile, err)
		}
	}

	var errs []error
	cfg := Config{
		Addr:                 getString("APP_ADDR", ":8080"),
		DatabaseDSN:          os.Getenv("DB_DSN"),
		AutoMigrate:          getBool("AUTO_MIGRATE", true, &errs),
		SessionTTL:           getDuration("SESSION_TTL", 30*24*time.Hour, &errs),
		SessionTouchInterval: getDuration("SESSION_TOUCH_INTERVAL", time.Hour, &errs),
		TrustProxy:           getBool("TRUST_PROXY", false, &errs),
		LoginRatePerMinute:   getFloat("LOGIN_RATE_PER_MINUTE", 5, &errs),
		LoginBurst:           getInt("LOGIN_BURST", 5, &errs),
		MaxBodyBytes:         int64(getInt("MAX_BODY_BYTES", 1<<20, &errs)),
		LogLevel:             getLogLevel("LOG_LEVEL", slog.LevelInfo, &errs),
	}

	if cfg.DatabaseDSN == "" {
		errs = append(errs, errors.New("DB_DSN is required"))
	}
	if cfg.SessionTTL <= 0 {
		errs = append(errs, errors.New("SESSION_TTL must be positive"))
	}
	if cfg.SessionTouchInterval < 0 {
		errs = append(errs, errors.New("SESSION_TOUCH_INTERVAL must not be negative"))
	}
	if cfg.LoginRatePerMinute <= 0 || cfg.LoginBurst <= 0 {
		errs = append(errs, errors.New("LOGIN_RATE_PER_MINUTE and LOGIN_BURST must be positive"))
	}
	if cfg.MaxBodyBytes <= 0 {
		errs = append(errs, errors.New("MAX_BODY_BYTES must be positive"))
	}

	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

func getString(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getBool(key string, def bool, errs *[]error) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return b
}

func getInt(key string, def int, errs *[]error) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return n
}

func getFloat(key string, def float64, errs *[]error) float64 {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return f
}

func getDuration(key string, def time.Duration, errs *[]error) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return d
}

func getLogLevel(key string, def slog.Level, errs *[]error) slog.Level {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	var l slog.Level
	if err := l.UnmarshalText([]byte(strings.ToUpper(v))); err != nil {
		*errs = append(*errs, fmt.Errorf("%s: %w", key, err))
		return def
	}
	return l
}
