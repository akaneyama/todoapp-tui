// Command server runs the todoapp API server.
//
// Usage:
//
//	server [-env .env]                  start the HTTP server
//	server [-env .env] migrate up       apply pending migrations
//	server [-env .env] migrate down     roll back the last migration
//	server [-env .env] migrate status   show migration status
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // embed IANA tz database (Windows / distroless images)

	"github.com/jmoiron/sqlx"

	"todoapp/internal/server/auth"
	"todoapp/internal/server/config"
	"todoapp/internal/server/db"
	"todoapp/internal/server/handler"
	"todoapp/internal/server/reminder"
	"todoapp/internal/server/store"
	"todoapp/internal/server/ws"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	envFile := flag.String("env", ".env", "optional .env file (real env vars take precedence)")
	flag.Parse()

	cfg, err := config.Load(*envFile)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	conn, err := db.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		return err
	}
	defer conn.Close()

	if args := flag.Args(); len(args) > 0 {
		if args[0] != "migrate" {
			return fmt.Errorf("unknown command %q", args[0])
		}
		return runMigrate(ctx, conn, log, args[1:])
	}

	if cfg.AutoMigrate {
		if err := db.MigrateUp(ctx, conn, log); err != nil {
			return err
		}
	}
	return serve(ctx, cfg, conn, log)
}

func serve(ctx context.Context, cfg config.Config, conn *sqlx.DB, log *slog.Logger) error {
	st := store.New(conn)
	authSvc, err := auth.NewService(st, auth.Options{
		SessionTTL:    cfg.SessionTTL,
		TouchInterval: cfg.SessionTouchInterval,
		Logger:        log,
	})
	if err != nil {
		return err
	}

	hub := ws.NewHub(log, st.ListRoomMembers)

	router := handler.NewRouter(handler.Deps{
		Auth:         authSvc,
		Todos:        st,
		Journal:      st,
		Chat:         st,
		UserLookup:   st,
		Hub:          hub,
		Health:       st,
		Logger:       log,
		TrustProxy:   cfg.TrustProxy,
		MaxBodyBytes: cfg.MaxBodyBytes,
		AuthRate:     handler.RateLimit{PerMinute: cfg.LoginRatePerMinute, Burst: cfg.LoginBurst},
	})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    16 << 10,
		// No BaseContext tied to ctx: in-flight requests must be allowed to
		// finish during Shutdown rather than being cancelled by the signal.
	}

	sched := reminder.NewScheduler(st, hub, log, 10*time.Second)

	bgCtx, cancelBg := context.WithCancel(ctx)
	defer cancelBg()
	go authSvc.RunJanitor(bgCtx, time.Hour)
	go sched.Start(bgCtx)

	errCh := make(chan error, 1)
	go func() {
		log.Info("server listening", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen: %w", err)
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	cancelBg()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	log.Info("server stopped")
	return nil
}

func runMigrate(ctx context.Context, conn *sqlx.DB, log *slog.Logger, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: server migrate up|down|status")
	}
	p, err := db.NewMigrator(conn)
	if err != nil {
		return err
	}
	switch args[0] {
	case "up":
		return db.MigrateUp(ctx, conn, log)
	case "down":
		r, err := p.Down(ctx)
		if err != nil {
			return fmt.Errorf("migrate down: %w", err)
		}
		log.Info("migration rolled back", "version", r.Source.Version, "file", r.Source.Path)
		return nil
	case "status":
		statuses, err := p.Status(ctx)
		if err != nil {
			return fmt.Errorf("migrate status: %w", err)
		}
		for _, s := range statuses {
			applied := "pending"
			if !s.AppliedAt.IsZero() {
				applied = s.AppliedAt.Format(time.RFC3339)
			}
			fmt.Printf("%5d  %-30s  %s\n", s.Source.Version, s.Source.Path, applied)
		}
		return nil
	default:
		return fmt.Errorf("unknown migrate command %q", args[0])
	}
}
