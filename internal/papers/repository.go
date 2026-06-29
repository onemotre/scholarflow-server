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

func (r *SQLRepository) CreatePaperUpload(ctx context.Context, info SourceInfo, asset storage.Object) (UploadResult, error) {
	paper, err := r.queries.CreatePaper(ctx, db.CreatePaperParams{
		SourceType:       info.SourceType,
		SourceID:         optString(info.SourceID),
		Status:           "queued",
		UploadedFilename: info.Filename,
		Title:            optString(info.Title),
		Abstract:         optString(info.Abstract),
		Doi:              optString(info.DOI),
		PublicationYear:  optInt32(info.Year),
		PrimaryCategory:  optString(info.PrimaryCategory),
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

func (r *SQLRepository) GetPaperBySourceID(ctx context.Context, sourceID string) (bool, error) {
	return r.queries.ExistsPaperBySource(ctx, optString(sourceID))
}

func optString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func optInt32(v int32) *int32 {
	if v == 0 {
		return nil
	}
	return &v
}
