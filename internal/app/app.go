package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/TBG-Chance/SentinelBox/internal/api"
	"github.com/TBG-Chance/SentinelBox/internal/buildinfo"
	"github.com/TBG-Chance/SentinelBox/internal/config"
	dbsqlite "github.com/TBG-Chance/SentinelBox/internal/database/sqlite"
)

type App struct {
	logger          *slog.Logger
	server          *http.Server
	store           *dbsqlite.Store
	shutdownTimeout time.Duration
	closeOnce       sync.Once
	closeErr        error
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger, build buildinfo.Info) (*App, error) {
	if ctx == nil {
		return nil, fmt.Errorf("application context is required")
	}
	if logger == nil {
		return nil, fmt.Errorf("application logger is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	store, err := dbsqlite.Open(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	handler := api.NewHandler(logger, store, build, cfg.Server.HealthTimeout.Duration)
	server := &http.Server{
		Addr:              cfg.Server.Listen,
		Handler:           handler,
		ReadHeaderTimeout: min(cfg.Server.ReadTimeout.Duration, 5*time.Second),
		ReadTimeout:       cfg.Server.ReadTimeout.Duration,
		WriteTimeout:      cfg.Server.WriteTimeout.Duration,
		IdleTimeout:       cfg.Server.IdleTimeout.Duration,
		MaxHeaderBytes:    64 << 10,
	}
	return &App{
		logger:          logger,
		server:          server,
		store:           store,
		shutdownTimeout: cfg.Server.ShutdownTimeout.Duration,
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	if a == nil || a.server == nil || a.store == nil {
		return fmt.Errorf("application is not initialized")
	}
	if ctx == nil {
		return fmt.Errorf("application context is required")
	}
	listener, err := net.Listen("tcp", a.server.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", a.server.Addr, err)
	}
	a.logger.Info("SentinelBox API listening", "address", listener.Addr().String())
	return a.serve(ctx, listener)
}

func (a *App) serve(ctx context.Context, listener net.Listener) error {
	if ctx == nil {
		return fmt.Errorf("application context is required")
	}
	if listener == nil {
		return fmt.Errorf("application listener is required")
	}
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- a.server.Serve(listener)
	}()

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), a.shutdownTimeout)
	defer cancel()
	if err := a.server.Shutdown(shutdownContext); err != nil {
		_ = a.server.Close()
		return fmt.Errorf("shut down HTTP server: %w", err)
	}
	if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP during shutdown: %w", err)
	}
	a.logger.Info("SentinelBox API stopped cleanly")
	return nil
}

func (a *App) Close() error {
	if a == nil {
		return nil
	}
	a.closeOnce.Do(func() {
		if a.store != nil {
			a.closeErr = a.store.Close()
		}
	})
	return a.closeErr
}
