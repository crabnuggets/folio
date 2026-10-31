package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/crabnuggets/folio/services/api/internal/auth"
	"github.com/crabnuggets/folio/services/api/internal/chatgpt"
	"github.com/crabnuggets/folio/services/api/internal/config"
	"github.com/crabnuggets/folio/services/api/internal/httpapi"
	"github.com/crabnuggets/folio/services/api/internal/store"
)

// main runs the API and exits with status 1 if startup, serving, or shutdown fails.
func main() {
	if err := run(); err != nil {
		slog.Error("API stopped", "error", err.Error())
		os.Exit(1)
	}
}

// run validates configuration, opens and migrates PostgreSQL, and serves the API
// until interrupted or terminated. It allows ten seconds to drain requests before
// forcing closure. Configuration, authentication, storage, and serving errors
// propagate; a failed shutdown returns a deadline error.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	middleware, err := auth.New(cfg.Auth)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	startup, stopStartup := context.WithTimeout(ctx, 20*time.Second)
	db, err := store.NewPostgres(startup, cfg.DatabaseURL)
	stopStartup()
	if err != nil {
		return err
	}
	defer db.Close()
	connection, err := chatgpt.New(db, cfg.OwnerID, cfg.ChatGPT)
	if err != nil {
		return err
	}
	handler, err := httpapi.NewWithWebDistDir(db, cfg.OwnerID, middleware, cfg.WebDistDir, connection)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: cfg.Address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	result := make(chan error, 1)
	go func() { slog.Info("API listening", "address", cfg.Address); result <- server.ListenAndServe() }()
	select {
	case err := <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdown, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelShutdown()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return errors.New("API shutdown deadline exceeded")
		}
		return nil
	}
}
