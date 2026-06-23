package jobs

import (
	"context"
	"log"
	"time"

	"github.com/hibiken/asynq"
)

type CleanupRepository interface {
	DeleteFailedJobsOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

type CleanupProcessor struct {
	repo          CleanupRepository
	retentionDays int
}

func NewCleanupProcessor(repo CleanupRepository, retentionDays int) *CleanupProcessor {
	return &CleanupProcessor{repo: repo, retentionDays: retentionDays}
}

func (p *CleanupProcessor) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeCleanupJobs, p.HandleCleanup)
}

func (p *CleanupProcessor) HandleCleanup(ctx context.Context, _ *asynq.Task) error {
	cutoff := time.Now().Add(-time.Duration(p.retentionDays) * 24 * time.Hour)
	deleted, err := p.repo.DeleteFailedJobsOlderThan(ctx, cutoff)
	if err != nil {
		return err
	}
	log.Printf("cleanup: deleted %d failed job(s) older than %d day(s)", deleted, p.retentionDays)
	return nil
}
