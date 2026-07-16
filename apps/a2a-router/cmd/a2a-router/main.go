package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/api"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/auth"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/config"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/ledger"
	"github.com/Nene7ko/NeKiro/apps/a2a-router/internal/resolution"
	a2atransport "github.com/Nene7ko/NeKiro/apps/a2a-router/internal/transport/a2a"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if err := run(context.Background(), os.Args[1:], logger); err != nil {
		logger.Error("a2a-router failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string, logger *slog.Logger) error {
	if len(arguments) == 0 {
		return errors.New("command is required: serve or migrate")
	}
	switch arguments[0] {
	case "serve":
		if len(arguments) != 1 {
			return errors.New("serve accepts no arguments")
		}
		return serve(ctx, logger)
	case "migrate":
		if len(arguments) != 2 || arguments[1] != "up" {
			return errors.New("migrate requires exactly one direction: up")
		}
		return migrate(ctx, arguments[1])
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}

func serve(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open Router Ledger database: %w", err)
	}
	defer pool.Close()
	ledgerStore, err := ledger.NewStore(pool)
	if err != nil {
		return err
	}
	if err := ledgerStore.Check(ctx); err != nil {
		return fmt.Errorf("Router Ledger schema is not ready: %w", err)
	}
	handler, err := newHandler(cfg, http.DefaultClient, http.DefaultClient, ledgerStore)
	if err != nil {
		return err
	}
	server := &http.Server{Addr: cfg.ListenAddress, Handler: handler}
	if logger != nil {
		logger.Info("a2a-router listening", "address", cfg.ListenAddress)
	}
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func migrate(ctx context.Context, direction string) (returnErr error) {
	databaseURL, err := config.LoadDatabaseURL()
	if err != nil {
		return err
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return errors.New("connect Router Ledger migration database")
	}
	defer func() {
		if closeErr := connection.Close(ctx); closeErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("close Router Ledger migration database: %w", closeErr))
		}
	}()
	if err := ledger.Migrate(ctx, connection, direction); err != nil {
		return errors.New("Router Ledger migration failed")
	}
	return nil
}

func newHandler(cfg config.Config, doer resolution.HTTPDoer, agentHTTPClient *http.Client, ledgerAppender api.InvocationLedgerAppender) (http.Handler, error) {
	authenticator, err := auth.NewStaticAuthenticator(cfg.RouterPrincipals)
	if err != nil {
		return nil, err
	}
	resolver, err := resolution.NewClient(doer, cfg.ControlPlaneResolveURL, cfg.ControlPlaneServiceToken, cfg.ControlPlaneResponseLimitBytes)
	if err != nil {
		return nil, err
	}
	transport, err := a2atransport.NewClient(agentHTTPClient, cfg.InternalRequestLimitBytes, cfg.AgentResponseLimitBytes)
	if err != nil {
		return nil, err
	}
	var dispatch *api.DispatchHandler
	if ledgerAppender == nil {
		return nil, errors.New("Router Ledger appender is required")
	}
	dispatch, err = api.NewDispatchHandlerWithTransportAndLedger(authenticator, resolver, transport, ledgerAppender, cfg.InternalRequestLimitBytes, cfg.ResolutionDeadline)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("GET /readyz", api.NewReadinessHandler())
	dispatch.RegisterRoutes(mux)
	return mux, nil
}
