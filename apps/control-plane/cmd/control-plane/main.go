package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog/postgres"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/config"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/gateway"
	"github.com/Nene7ko/NeKiro/contracts"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(context.Background(), os.Args[1:], logger); err != nil {
		logger.Error("control plane stopped", "error", publicCommandError(err))
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, logger *slog.Logger) error {
	if len(arguments) == 0 {
		return errors.New("command is required: serve, migrate, or healthcheck")
	}
	switch arguments[0] {
	case "serve":
		if len(arguments) != 1 {
			return errors.New("serve accepts no arguments")
		}
		return serve(ctx, logger)
	case "migrate":
		if len(arguments) != 2 {
			return errors.New("migrate requires exactly one direction: up or down")
		}
		return migrate(ctx, arguments[1])
	case "healthcheck":
		if len(arguments) != 2 {
			return errors.New("healthcheck requires exactly one URL")
		}
		return healthcheck(ctx, arguments[1])
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}

func serve(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	authenticator, err := gateway.NewDevelopmentStaticAuthenticator(cfg.Principals)
	if err != nil {
		return fmt.Errorf("initialize authenticator: %w", err)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return errors.New("initialize database pool")
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return errors.New("connect database dependency")
	}
	store, err := postgres.NewStore(pool)
	if err != nil {
		return err
	}
	if err := store.Check(ctx); err != nil {
		return errors.New("catalog schema is not ready")
	}
	validator, err := contracts.NewValidator()
	if err != nil {
		return errors.New("initialize contract validator")
	}
	catalogService, err := catalog.NewService(store, validator, time.Now)
	if err != nil {
		return err
	}
	traces, err := gateway.NewTraceGenerator()
	if err != nil {
		return err
	}
	handler, err := gateway.NewHandler(authenticator, catalogService, store, traces, logger)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	shutdownContext, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("control plane listening", "address", cfg.ListenAddress)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve control plane: %w", err)
	case <-shutdownContext.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return fmt.Errorf("shutdown control plane: %w", err)
		}
		return nil
	}
}

func migrate(ctx context.Context, direction string) error {
	databaseURL, err := config.LoadDatabaseURL()
	if err != nil {
		return err
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return errors.New("connect migration database")
	}
	defer connection.Close(ctx)
	if err := postgres.Migrate(ctx, connection, direction); err != nil {
		return errors.New("catalog migration failed")
	}
	return nil
}

func healthcheck(ctx context.Context, url string) error {
	requestContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, url, nil)
	if err != nil {
		return errors.New("healthcheck URL is invalid")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return errors.New("healthcheck request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("healthcheck returned status %d", response.StatusCode)
	}
	return nil
}

func publicCommandError(err error) string {
	return err.Error()
}
