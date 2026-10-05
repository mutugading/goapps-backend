// Package httpdelivery provides the HTTP server for the gRPC gateway, metrics,
// and health checks.
package httpdelivery

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/payroll/internal/infrastructure/config"
)

// Server represents the HTTP server.
//
// It does not yet proxy any gRPC-Gateway routes or serve Swagger -- those are
// wired once `payroll/v1` proto exists (T004). For now it only exposes health,
// readiness, liveness and metrics so the service is observable standalone.
type Server struct {
	server         *http.Server
	config         *config.ServerConfig
	allowedOrigins []string
	corsMaxAge     int
}

// NewServer creates a new HTTP server.
func NewServer(cfg *config.ServerConfig, opts ...Option) *Server {
	s := &Server{
		config:         cfg,
		allowedOrigins: []string{"http://localhost:3000"},
		corsMaxAge:     300,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Option configures the HTTP server.
type Option func(*Server)

// WithCORS sets CORS allowed origins and max age.
func WithCORS(origins []string, maxAge int) Option {
	return func(s *Server) {
		if len(origins) > 0 {
			s.allowedOrigins = origins
		}
		if maxAge > 0 {
			s.corsMaxAge = maxAge
		}
	}
}

// Start starts the HTTP server.
func (s *Server) Start(_ context.Context) error {
	mux := http.NewServeMux()

	// Health check endpoints.
	mux.HandleFunc("/healthz", s.healthHandler)
	mux.HandleFunc("/readyz", s.readyHandler)
	mux.HandleFunc("/livez", s.liveHandler)

	// Metrics endpoint.
	mux.Handle("/metrics", promhttp.Handler())

	// CORS middleware (configurable for SSO multi-app support).
	corsHandler := cors.New(cors.Options{
		AllowedOrigins:   s.allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "X-Requested-With"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           s.corsMaxAge,
	}).Handler(mux)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.HTTPPort),
		Handler:      corsHandler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Info().
		Int("port", s.config.HTTPPort).
		Msg("HTTP server starting")

	return s.server.ListenAndServe()
}

// Stop stops the HTTP server.
func (s *Server) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

func (s *Server) healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"healthy"}`)); err != nil {
		log.Warn().Err(err).Msg("Failed to write health response")
	}
}

func (s *Server) readyHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"ready"}`)); err != nil {
		log.Warn().Err(err).Msg("Failed to write ready response")
	}
}

func (s *Server) liveHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(`{"status":"live"}`)); err != nil {
		log.Warn().Err(err).Msg("Failed to write live response")
	}
}
