package papers

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/google/uuid"

	"scholarflow_server/internal/storage"
)

type Repository interface {
	CreatePaperUpload(ctx context.Context, filename string, asset storage.Object) (UploadResult, error)
	SetJobTaskID(ctx context.Context, jobID uuid.UUID, taskID string) error
}

type Enqueuer interface {
	EnqueuePaperProcessing(ctx context.Context, paperID uuid.UUID, jobID uuid.UUID) (string, error)
}

type Service struct {
	repo     Repository
	store    storage.Store
	enqueuer Enqueuer
}

func NewService(repo Repository, store storage.Store, enqueuer Enqueuer) *Service {
	return &Service{repo: repo, store: store, enqueuer: enqueuer}
}

func (s *Service) UploadPDF(ctx context.Context, filename string, body io.Reader, size int64, contentType string) (UploadResult, error) {
	if contentType != "application/pdf" {
		return UploadResult{}, fmt.Errorf("unsupported content type: %s", contentType)
	}
	if size <= 0 {
		return UploadResult{}, fmt.Errorf("empty upload")
	}
	key := fmt.Sprintf("papers/%s/%s", uuid.NewString(), filepath.Base(filename))
	asset, err := s.store.Put(ctx, key, body, size, contentType)
	if err != nil {
		return UploadResult{}, err
	}
	result, err := s.repo.CreatePaperUpload(ctx, filename, asset)
	if err != nil {
		return UploadResult{}, err
	}
	taskID, err := s.enqueuer.EnqueuePaperProcessing(ctx, result.PaperID, result.JobID)
	if err != nil {
		return UploadResult{}, err
	}
	if err := s.repo.SetJobTaskID(ctx, result.JobID, taskID); err != nil {
		return UploadResult{}, err
	}
	return result, nil
}
