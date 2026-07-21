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
	catalogpostgres "github.com/Nene7ko/NeKiro/apps/control-plane/internal/catalog/postgres"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/config"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/gateway"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/invocation"
	"github.com/Nene7ko/NeKiro/apps/control-plane/internal/workspace"
	workspacepostgres "github.com/Nene7ko/NeKiro/apps/control-plane/internal/workspace/postgres"
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
		if len(arguments) != 2 || arguments[1] != "up" {
			return errors.New("migrate requires exactly one direction: up")
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
	invocationConfig, err := config.LoadInvocationRuntime()
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
	catalogStore, err := catalogpostgres.NewStore(pool)
	if err != nil {
		return err
	}
	workspaceStore, err := workspacepostgres.NewStore(pool)
	if err != nil {
		return err
	}
	if err := catalogStore.Check(ctx); err != nil {
		return errors.New("catalog schema is not ready")
	}
	if err := workspaceStore.Check(ctx); err != nil {
		return errors.New("workspace schema is not ready")
	}
	validator, err := contracts.NewValidator()
	if err != nil {
		return errors.New("initialize contract validator")
	}
	catalogService, err := catalog.NewService(catalogStore, validator, time.Now)
	if err != nil {
		return err
	}
	internalAuthenticator, err := gateway.NewDevelopmentStaticAuthenticator(cfg.InternalPrincipals)
	if err != nil {
		return fmt.Errorf("initialize internal authenticator: %w", err)
	}
	workspaceService, err := workspace.NewService(workspaceStore, catalogService, workspace.NewOwnerPolicy(), validator, time.Now, workspace.NewRandomID)
	if err != nil {
		return err
	}
	traces, err := gateway.NewTraceGenerator()
	if err != nil {
		return err
	}
	readiness := combinedReadiness{catalog: catalogStore, workspace: workspaceStore}
	catalogHandler, err := gateway.NewHandler(authenticator, catalogService, readiness, traces, logger)
	if err != nil {
		return err
	}
	workspaceHandler, err := gateway.NewWorkspaceHandler(authenticator, internalAuthenticator, workspaceService, traces, logger, invocationConfig.InternalRequestLimitBytes)
	if err != nil {
		return err
	}
	routerClient, err := invocation.NewRouterClient(newRouterHTTPClient(), invocationConfig.RouterInternalURL, invocationConfig.RouterBearerToken)
	if err != nil {
		return err
	}
	invocationService, err := invocation.NewService(workspaceService, routerClient, invocation.NewRandomIDGenerator())
	if err != nil {
		return err
	}
	metadataReadService, err := invocation.NewMetadataReadService(workspaceService, routerClient)
	if err != nil {
		return err
	}
	invocationHandler, err := gateway.NewInvocationHandler(authenticator, invocationService, traces, logger, invocationConfig.PublicRequestLimitBytes, invocationConfig.SSEEventLimitBytes, time.Duration(invocationConfig.DeadlineMS)*time.Millisecond)
	if err != nil {
		return err
	}
	invocationReadHandler, err := gateway.NewInvocationReadHandler(authenticator, metadataReadService, traces, logger, time.Duration(invocationConfig.DeadlineMS)*time.Millisecond, invocationConfig.MetadataResponseLimitBytes)
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	catalogHandler.RegisterRoutes(mux)
	workspaceHandler.RegisterRoutes(mux)
	invocationHandler.RegisterRoutes(mux)
	invocationReadHandler.RegisterRoutes(mux)

	server := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           gateway.CORS(cfg.CORSAllowedOrigins, mux),
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

// newRouterHTTPClient performs exactly one request to the configured Router
// destination. Redirects would otherwise cause net/http to issue a second
// request, potentially to an alternate origin, while preserving the original
// bearer credential. The Router boundary has no redirect policy, so surface
// the response to the caller instead.
func newRouterHTTPClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func migrate(ctx context.Context, direction string) (returnErr error) {
	databaseURL, err := config.LoadDatabaseURL()
	if err != nil {
		return err
	}
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return errors.New("connect migration database")
	}
	defer func() {
		if closeErr := connection.Close(ctx); closeErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("close migration database: %w", closeErr))
		}
	}()
	if err := catalogpostgres.Migrate(ctx, connection, direction); err != nil {
		return errors.New("catalog migration failed")
	}
	if err := workspacepostgres.Migrate(ctx, connection, direction); err != nil {
		return errors.New("workspace migration failed")
	}
	return nil
}

type combinedReadiness struct {
	catalog   gateway.ReadinessChecker
	workspace gateway.ReadinessChecker
}

func (readiness combinedReadiness) Check(ctx context.Context) error {
	if err := readiness.catalog.Check(ctx); err != nil {
		return err
	}
	return readiness.workspace.Check(ctx)
}

func healthcheck(ctx context.Context, url string) (returnErr error) {
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
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("close healthcheck response: %w", closeErr))
		}
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("healthcheck returned status %d", response.StatusCode)
	}
	return nil
}

func publicCommandError(err error) string {
	return err.Error()
}
