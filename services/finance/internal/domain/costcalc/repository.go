package costcalc

import "context"

// JobFilter describes optional filters for ListJobs.
type JobFilter struct {
	Period      string
	CalcType    CalculationType
	Status      JobStatus
	TriggeredBy string
	Page        int
	PageSize    int
}

// JobProductFilter describes optional filters for ListJobProducts.
type JobProductFilter struct {
	Status   JobProductStatus
	Page     int
	PageSize int
}

// JobRepository persists Job aggregates.
type JobRepository interface {
	Create(ctx context.Context, j *Job) error
	GetByID(ctx context.Context, id int64) (*Job, error)
	List(ctx context.Context, f JobFilter) ([]*Job, int, error)
	UpdateStatus(ctx context.Context, id int64, status JobStatus) error
	UpdateTotals(ctx context.Context, id int64, total, totalChunks, totalWaves int) error
	UpdateProgress(ctx context.Context, id int64, processed, succ, fail, blocked int) error
	UpdateCompletion(ctx context.Context, id int64, status JobStatus, succ, fail, blocked int, durationMs int64, errSummary []byte) error
}

// ChunkRepository persists Chunk aggregates.
type ChunkRepository interface {
	Create(ctx context.Context, c *Chunk) error
	GetByID(ctx context.Context, id int64) (*Chunk, error)
	ListByJob(ctx context.Context, jobID int64, wave *int, status *ChunkStatus, page, pageSize int) ([]*Chunk, int, error)
	UpdateStatus(ctx context.Context, id int64, status ChunkStatus, workerID string) error
	UpdateResult(ctx context.Context, id int64, status ChunkStatus, succ, fail, durationMs int, errMsg string) error
	IncrementRetry(ctx context.Context, id int64) (int, error)
}

// JobProductRepository persists JobProduct aggregates.
type JobProductRepository interface {
	BulkCreate(ctx context.Context, items []*JobProduct) error
	GetByJobAndProduct(ctx context.Context, jobID, productSysID int64) (*JobProduct, error)
	ListByJob(ctx context.Context, jobID int64, f JobProductFilter) ([]*JobProduct, int, error)
	AssignChunk(ctx context.Context, jobID, productSysID, chunkID int64) error
	MarkSuccess(ctx context.Context, jobID, productSysID, costID int64, durationMs int, log []byte) error
	MarkFailed(ctx context.Context, jobID, productSysID int64, errMsg string, log []byte) error
	MarkBlocked(ctx context.Context, jobID, productSysID int64, reason string, log []byte) error
	MarkSkippedForJob(ctx context.Context, jobID int64) error
}

// ResultRepository persists Result aggregates.
type ResultRepository interface {
	// UpsertWithSupersede SUPERSEDEs the existing active row (if any) and inserts a new one
	// atomically. Returns (newCostID, prevVersion, prevTotal, prevCostID).
	UpsertWithSupersede(ctx context.Context, r *Result) (newCostID int64, prevVersion int, prevTotal float64, prevCostID int64, err error)
	GetActive(ctx context.Context, productSysID int64, period string, calcType CalculationType) (*Result, error)
	GetByID(ctx context.Context, id int64) (*Result, error)
	ListHistory(ctx context.Context, productSysID int64, calcType CalculationType, page, pageSize int) ([]*Result, int, error)
	MarkVerified(ctx context.Context, costID int64, by string) error
	MarkApproved(ctx context.Context, costID int64, by string) error
}

// AuditHistoryRepository persists AuditHistoryEntry rows.
type AuditHistoryRepository interface {
	Write(ctx context.Context, e *AuditHistoryEntry) error
}
