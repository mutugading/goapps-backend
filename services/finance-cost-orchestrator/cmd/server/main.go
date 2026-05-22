// Package main is the entry point for the finance-cost-orchestrator service.
//
// Bootstrap responsibilities:
//   - Load config (viper)
//   - Configure zerolog
//   - Establish RabbitMQ connection (with retry so failures surface quickly)
//   - Expose Prometheus /metrics + /healthz
//   - Start the Coordinator main loop
//   - Handle SIGINT/SIGTERM for graceful shutdown
//
// The actual planner / publisher / chunk-coordinator logic lands in S8c.2 - S8c.5.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance-cost-orchestrator/internal/config"
	"github.com/mutugading/goapps-backend/services/finance-cost-orchestrator/internal/infrastructure/rmq"
	"github.com/mutugading/goapps-backend/services/finance-cost-orchestrator/internal/orchestrator"
)

func main() {
	if err := run(); err != nil {
		log.Fatal().Err(err).Msg("orchestrator failed")
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	setupLogger(cfg)

	log.Info().
		Str("service", cfg.App.Name).
		Str("version", cfg.App.Version).
		Str("environment", cfg.App.Env).
		Msg("Starting finance-cost-orchestrator")

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// RabbitMQ connection (with bounded retry so we exit fast when RMQ is down).
	rmqConn, err := rmq.ConnectWithRetry(cfg.RabbitMQ.URL, 3, cfg.RabbitMQ.ReconnectDelay)
	if err != nil {
		return fmt.Errorf("connect rabbitmq: %w", err)
	}
	defer func() {
		if closeErr := rmqConn.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("close rabbitmq")
		}
	}()
	log.Info().Msg("RabbitMQ connected")

	// DB connection deferred to S8c.5 (orchestrator repos).

	// Start coordinator loop.
	coord := orchestrator.New(cfg, rmqConn)
	coordErrCh := make(chan error, 1)
	go func() { coordErrCh <- coord.Run(ctx) }()

	// HTTP server for /metrics + /healthz.
	srv := newHTTPServer(cfg.Server.MetricsPort)
	srvErrCh := make(chan error, 1)
	go func() {
		log.Info().Str("addr", srv.Addr).Msg("metrics+health server listening")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErrCh <- err
			return
		}
		srvErrCh <- nil
	}()

	// Wait for shutdown signal or fatal error.
	select {
	case <-ctx.Done():
		log.Info().Msg("shutdown signal received")
	case err := <-coordErrCh:
		if err != nil {
			log.Error().Err(err).Msg("coordinator exited with error")
			cancel()
			return err
		}
	case err := <-srvErrCh:
		if err != nil {
			log.Error().Err(err).Msg("metrics server exited with error")
			cancel()
			return err
		}
	}

	// Graceful shutdown.
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Warn().Err(err).Msg("metrics server shutdown")
	}

	log.Info().Msg("orchestrator stopped")
	return nil
}

func setupLogger(cfg *config.Config) {
	zerolog.TimeFieldFormat = time.RFC3339
	switch cfg.Logger.Level {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	if cfg.App.Env == "development" {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}
}

func newHTTPServer(port int) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
}
