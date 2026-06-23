package papers

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ErrNotRetryable is returned when a job cannot be retried (not in 'failed' state).
var ErrNotRetryable = errors.New("job is not in a retryable state")

type RetryRepository interface {
	GetJob(ctx context.Context, jobID uuid.UUID) (JobStatus, error)
	CountPaperSections(ctx context.Context, paperID uuid.UUID) (int64, error)
	ResetFailedJob(ctx context.Context, jobID uuid.UUID) (int64, error)
	SetJobTaskID(ctx context.Context, jobID uuid.UUID, taskID string) error
}

type RetryEnqueuer interface {
	EnqueuePaperRead(ctx context.Context, paperID, jobID uuid.UUID) (string, error)
	EnqueuePaperProcessing(ctx context.Context, paperID, jobID uuid.UUID) (string, error)
}

type RetryService struct {
	repo     RetryRepository
	enqueuer RetryEnqueuer
}

func NewRetryService(repo RetryRepository, enqueuer RetryEnqueuer) *RetryService {
	return &RetryService{repo: repo, enqueuer: enqueuer}
}

// RetryJob resets a failed job and re-enqueues the stage inferred from paper
// state (parsed -> read, otherwise -> parse). Returns ErrNotRetryable when the
// job is not currently 'failed'.
func (s *RetryService) RetryJob(ctx context.Context, jobID uuid.UUID) (JobStatus, error) {
	job, err := s.repo.GetJob(ctx, jobID)
	if err != nil {
		return JobStatus{}, err
	}
	if job.Status != "failed" {
		return JobStatus{}, ErrNotRetryable
	}
	rows, err := s.repo.ResetFailedJob(ctx, jobID)
	if err != nil {
		return JobStatus{}, err
	}
	if rows == 0 {
		return JobStatus{}, ErrNotRetryable
	}
	sectionCount, err := s.repo.CountPaperSections(ctx, job.PaperID)
	if err != nil {
		return JobStatus{}, err
	}
	var taskID string
	if sectionCount > 0 {
		taskID, err = s.enqueuer.EnqueuePaperRead(ctx, job.PaperID, jobID)
	} else {
		taskID, err = s.enqueuer.EnqueuePaperProcessing(ctx, job.PaperID, jobID)
	}
	if err != nil {
		return JobStatus{}, err
	}
	if err := s.repo.SetJobTaskID(ctx, jobID, taskID); err != nil {
		return JobStatus{}, err
	}
	return s.repo.GetJob(ctx, jobID)
}
