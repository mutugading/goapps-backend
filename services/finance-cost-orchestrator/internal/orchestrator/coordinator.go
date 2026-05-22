// Package orchestrator coordinates calc-job execution: planning chunks,
// publishing them to worker queues, consuming chunk-done events, and finalizing
// the job. The bootstrap exposes only the lifecycle skeleton — real planner
// (S8c.2), publisher (S8c.4), and coordinator state machine (S8c.5) land later.
package orchestrator

import (
	"context"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance-cost-orchestrator/internal/config"
	"github.com/mutugading/goapps-backend/services/finance-cost-orchestrator/internal/infrastructure/rmq"
)

// Coordinator is the top-level orchestrator runtime. Wired in main and started
// as a goroutine.
type Coordinator struct {
	cfg     *config.Config
	rmqConn *rmq.Connection
}

// New constructs a Coordinator with its required dependencies. Additional deps
// (DB repos, publisher, consumer) will be injected in S8c.5+.
func New(cfg *config.Config, rmqConn *rmq.Connection) *Coordinator {
	return &Coordinator{cfg: cfg, rmqConn: rmqConn}
}

// Run blocks until the context is cancelled. In S8c.5 this will start the
// planner + consumer loops; for the bootstrap it just idles so the binary
// stays up and signal handling can shut it down cleanly.
func (c *Coordinator) Run(ctx context.Context) error {
	log.Info().
		Int("chunk_size", c.cfg.Orchestrator.ChunkSize).
		Int("max_chunk_size", c.cfg.Orchestrator.MaxChunkSize).
		Msg("Orchestrator coordinator started (bootstrap — no work yet)")

	<-ctx.Done()

	log.Info().Msg("Orchestrator coordinator stopping")
	return nil
}
