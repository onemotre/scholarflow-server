package papers

import (
	"context"

	"github.com/google/uuid"

	"scholarflow_server/internal/db"
	"scholarflow_server/internal/storage"
)

type SQLRepository struct {
	queries *db.Queries
}

func NewSQLRepository(queries *db.Queries) *SQLRepository {
	return &SQLRepository{queries: queries}
}

func (r *SQLRepository) CreatePaperUpload(ctx context.Context, filename string, asset storage.Object) (UploadResult, error) {
	paper, err := r.queries.CreatePaper(ctx, db.CreatePaperParams{
		SourceType:       "local_pdf",
		Status:           "queued",
		UploadedFilename: filename,
	})
	if err != nil {
		return UploadResult{}, err
	}
	_, err = r.queries.CreatePaperAsset(ctx, db.CreatePaperAssetParams{
		PaperID:       paper.ID,
		AssetType:     "pdf",
		StorageBucket: asset.Bucket,
		StorageKey:    asset.Key,
		ContentType:   asset.ContentType,
		SizeBytes:     asset.SizeBytes,
		Checksum:      &asset.Checksum,
	})
	if err != nil {
		return UploadResult{}, err
	}
	job, err := r.queries.CreateProcessingJob(ctx, db.CreateProcessingJobParams{
		PaperID: paper.ID,
		Status:  "queued",
	})
	if err != nil {
		return UploadResult{}, err
	}
	return UploadResult{PaperID: uuid.UUID(paper.ID), JobID: uuid.UUID(job.ID)}, nil
}

func (r *SQLRepository) SetJobTaskID(ctx context.Context, jobID uuid.UUID, taskID string) error {
	_, err := r.queries.SetProcessingJobTaskID(ctx, db.SetProcessingJobTaskIDParams{
		ID:     jobID,
		TaskID: &taskID,
	})
	return err
}
