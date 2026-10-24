// Package main is the entry point for the Payroll service.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	payrollv1 "github.com/mutugading/goapps-backend/gen/payroll/v1"
	masterapp "github.com/mutugading/goapps-backend/services/payroll/internal/application/master"
	grpcdelivery "github.com/mutugading/goapps-backend/services/payroll/internal/delivery/grpc"
	httpdelivery "github.com/mutugading/goapps-backend/services/payroll/internal/delivery/httpdelivery"
	auditinfra "github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/audit"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/tracing"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("Service failed")
	}
}

// run contains the main application logic, separated for cleaner error handling.
func run() error {
	setupLogger()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log.Info().
		Str("service", cfg.App.Name).
		Str("version", cfg.App.Version).
		Str("environment", cfg.App.Env).
		Msg("Starting Payroll service")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup tracing (optional).
	cleanupTracing := setupTracing(ctx, cfg)
	defer cleanupTracing()

	return startServers(ctx, cfg)
}

// setupLogger configures the application logger.
func setupLogger() {
	zerolog.TimeFieldFormat = time.RFC3339
	if os.Getenv("APP_ENV") == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}

// setupTracing initializes tracing and returns a cleanup function.
func setupTracing(ctx context.Context, cfg *config.Config) func() {
	tracingProvider, err := tracing.NewProvider(ctx, &cfg.Tracing, cfg.App.Name, cfg.App.Version)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to setup tracing, continuing without it")
		return func() {}
	}

	if tracingProvider == nil {
		return func() {}
	}

	return func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := tracingProvider.Shutdown(shutdownCtx); err != nil {
			log.Warn().Err(err).Msg("Failed to shutdown tracing provider")
		}
	}
}

// startServers starts the gRPC and HTTP servers and handles graceful shutdown.
func startServers(ctx context.Context, cfg *config.Config) error {
	db, err := postgres.NewConnection(&cfg.Database)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			log.Warn().Err(cerr).Msg("Failed to close database")
		}
	}()

	idemStore := postgres.NewIdempotencyStore(db)
	go idemStore.RunPurgeLoop(ctx, time.Hour, 24*time.Hour) // expired Idempotency-Keys (D-19, T007f)

	grpcServer, err := grpcdelivery.NewServer(&cfg.Server, &cfg.JWT, postgres.NewScopeResolver(db), idemStore)
	if err != nil {
		return err
	}

	masterSvc := masterapp.NewService(db, postgres.NewMasterRepository(), auditinfra.NewWriter(), cfg.PII.EncKey, cfg.PII.HMACKey).WithComponents(postgres.NewComponentRepository()).WithAttachments(postgres.NewAttachmentRepository())
	payrollv1.RegisterPayrollServiceServer(grpcServer.GRPCServer(), grpcdelivery.NewMasterHandler(masterSvc))

	// Start gRPC server.
	go func() {
		if err := grpcServer.Start(); err != nil {
			log.Error().Err(err).Msg("gRPC server failed")
		}
	}()

	// Start HTTP server (health, readiness, liveness, metrics) with CORS config.
	httpServer := httpdelivery.NewServer(&cfg.Server,
		httpdelivery.WithCORS(cfg.CORS.AllowedOrigins, cfg.CORS.MaxAge),
	)
	go func() {
		if err := httpServer.Start(ctx); err != nil {
			log.Warn().Err(err).Msg("HTTP server stopped")
		}
	}()

	// Wait for shutdown signal.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down servers...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}

	grpcServer.Stop()

	log.Info().Msg("Server shutdown complete")
	return nil
}
