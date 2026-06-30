package papers

import (
	"context"
	"log"

	"github.com/google/uuid"
)

// AdminRepository is the persistence surface the AdminService needs.
type AdminRepository interface {
	ListPaperAssets(ctx context.Context, paperID uuid.UUID) ([]AssetRef, error)
	DeletePaper(ctx context.Context, paperID uuid.UUID) (int64, error)
	GetLatestJobByPaper(ctx context.Context, paperID uuid.UUID) (JobStatus, error)
	RequeueJob(ctx context.Context, jobID uuid.UUID) (int64, error)
	CountPaperSections(ctx context.Context, paperID uuid.UUID) (int64, error)
	GetJob(ctx context.Context, jobID uuid.UUID) (JobStatus, error)
	SetJobTaskID(ctx context.Context, jobID uuid.UUID, taskID string) error
}

// AdminStore is the object-store surface (delete only) the AdminService needs.
type AdminStore interface {
	Delete(ctx context.Context, key string) error
}

// AdminEnqueuer re-enqueues pipeline stages.
type AdminEnqueuer interface {
	EnqueuePaperProcessing(ctx context.Context, paperID, jobID uuid.UUID) (string, error)
	EnqueuePaperRead(ctx context.Context, paperID, jobID uuid.UUID) (string, error)
}

type AdminService struct {
	repo     AdminRepository
	store    AdminStore
	enqueuer AdminEnqueuer
}

func NewAdminService(repo AdminRepository, store AdminStore, enqueuer AdminEnqueuer) *AdminService {
	return &AdminService{repo: repo, store: store, enqueuer: enqueuer}
}

// DeletePaper hard-deletes a paper: it collects the paper's stored objects,
// deletes the DB row (cascading all child rows), then best-effort removes the
// objects from the store. The DB is the source of truth, so an object-delete
// failure is logged but does not fail the request. Returns ErrNotFound when no
// row was deleted.
func (s *AdminService) DeletePaper(ctx context.Context, paperID uuid.UUID) error {
	assets, err := s.repo.ListPaperAssets(ctx, paperID)
	if err != nil {
		return err
	}
	rows, err := s.repo.DeletePaper(ctx, paperID)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	for _, a := range assets {
		if derr := s.store.Delete(ctx, a.Key); derr != nil {
			log.Printf("admin: failed to delete object %q for paper %s: %v", a.Key, paperID, derr)
		}
	}
	return nil
}

// Reprocess requeues the parse stage for a paper regardless of current job
// state. Returns ErrNotFound when the paper has no job.
func (s *AdminService) Reprocess(ctx context.Context, paperID uuid.UUID) (JobStatus, error) {
	job, err := s.repo.GetLatestJobByPaper(ctx, paperID)
	if err != nil {
		return JobStatus{}, err
	}
	if _, err := s.repo.RequeueJob(ctx, job.JobID); err != nil {
		return JobStatus{}, err
	}
	taskID, err := s.enqueuer.EnqueuePaperProcessing(ctx, paperID, job.JobID)
	if err != nil {
		return JobStatus{}, err
	}
	if err := s.repo.SetJobTaskID(ctx, job.JobID, taskID); err != nil {
		return JobStatus{}, err
	}
	return s.repo.GetJob(ctx, job.JobID)
}

// RegenerateCard requeues the read stage to rebuild the paper card. Returns
// ErrNotFound when the paper has no job, and ErrNotRetryable when the paper has
// not been parsed yet (no sections to read).
func (s *AdminService) RegenerateCard(ctx context.Context, paperID uuid.UUID) (JobStatus, error) {
	job, err := s.repo.GetLatestJobByPaper(ctx, paperID)
	if err != nil {
		return JobStatus{}, err
	}
	count, err := s.repo.CountPaperSections(ctx, paperID)
	if err != nil {
		return JobStatus{}, err
	}
	if count == 0 {
		return JobStatus{}, ErrNotRetryable
	}
	if _, err := s.repo.RequeueJob(ctx, job.JobID); err != nil {
		return JobStatus{}, err
	}
	taskID, err := s.enqueuer.EnqueuePaperRead(ctx, paperID, job.JobID)
	if err != nil {
		return JobStatus{}, err
	}
	if err := s.repo.SetJobTaskID(ctx, job.JobID, taskID); err != nil {
		return JobStatus{}, err
	}
	return s.repo.GetJob(ctx, job.JobID)
}
