package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"scholarflow_server/internal/db"
	"scholarflow_server/internal/parser"
	"scholarflow_server/internal/storage"
)

type SQLRepository struct {
	queries *db.Queries
}

func NewSQLRepository(queries *db.Queries) *SQLRepository {
	return &SQLRepository{queries: queries}
}

func (r *SQLRepository) UpdateJobStatus(ctx context.Context, jobID uuid.UUID, status string, errorMessage *string, attemptIncrement int32) error {
	_, err := r.queries.UpdateProcessingJobStatus(ctx, db.UpdateProcessingJobStatusParams{
		ID:           jobID,
		Status:       status,
		ErrorMessage: errorMessage,
		AttemptCount: attemptIncrement,
	})
	return err
}

func (r *SQLRepository) GetPaperPDFAsset(ctx context.Context, paperID uuid.UUID) (storage.Object, error) {
	asset, err := r.queries.GetPaperAssetByType(ctx, db.GetPaperAssetByTypeParams{
		PaperID:   paperID,
		AssetType: "pdf",
	})
	if err != nil {
		return storage.Object{}, err
	}
	return storage.Object{
		Bucket:      asset.StorageBucket,
		Key:         asset.StorageKey,
		ContentType: asset.ContentType,
		SizeBytes:   asset.SizeBytes,
		Checksum:    stringValue(asset.Checksum),
	}, nil
}

func (r *SQLRepository) CreateTEIAsset(ctx context.Context, paperID uuid.UUID, asset storage.Object) error {
	_, err := r.queries.CreatePaperAsset(ctx, db.CreatePaperAssetParams{
		PaperID:       paperID,
		AssetType:     "grobid_tei_xml",
		StorageBucket: asset.Bucket,
		StorageKey:    asset.Key,
		ContentType:   asset.ContentType,
		SizeBytes:     asset.SizeBytes,
		Checksum:      stringPointer(asset.Checksum),
	})
	return err
}

func (r *SQLRepository) SaveParsedPaper(ctx context.Context, paperID uuid.UUID, parsed parser.ParsedPaper) error {
	_, err := r.queries.UpdatePaperMetadata(ctx, db.UpdatePaperMetadataParams{
		ID:              paperID,
		Title:           stringPointer(parsed.Title),
		Abstract:        stringPointer(parsed.Abstract),
		Doi:             stringPointer(parsed.DOI),
		PublicationYear: int32Pointer(parsed.Year),
		Status:          StatusParsed,
	})
	if err != nil {
		return fmt.Errorf("update paper metadata: %w", err)
	}
	if err := r.queries.DeletePaperAuthors(ctx, paperID); err != nil {
		return fmt.Errorf("delete paper authors: %w", err)
	}
	for _, author := range parsed.Authors {
		_, err := r.queries.CreatePaperAuthor(ctx, db.CreatePaperAuthorParams{
			PaperID:     paperID,
			AuthorOrder: author.Order,
			DisplayName: author.DisplayName,
			Orcid:       stringPointer(author.ORCID),
		})
		if err != nil {
			return fmt.Errorf("create paper author: %w", err)
		}
	}
	if err := r.queries.DeletePaperSections(ctx, paperID); err != nil {
		return fmt.Errorf("delete paper sections: %w", err)
	}
	for _, section := range parsed.Sections {
		_, err := r.queries.CreatePaperSection(ctx, db.CreatePaperSectionParams{
			PaperID:      paperID,
			SectionOrder: section.Order,
			Heading:      stringPointer(section.Heading),
			Text:         section.Text,
			PageStart:    section.PageStart,
			PageEnd:      section.PageEnd,
			GrobidPath:   stringPointer(section.Anchor),
		})
		if err != nil {
			return fmt.Errorf("create paper section: %w", err)
		}
	}
	if err := r.queries.DeletePaperReferences(ctx, paperID); err != nil {
		return fmt.Errorf("delete paper references: %w", err)
	}
	for _, reference := range parsed.References {
		authors, err := json.Marshal(reference.Authors)
		if err != nil {
			return fmt.Errorf("marshal reference authors: %w", err)
		}
		_, err = r.queries.CreatePaperReference(ctx, db.CreatePaperReferenceParams{
			PaperID:        paperID,
			ReferenceOrder: reference.Order,
			Title:          stringPointer(reference.Title),
			Authors:        authors,
			Venue:          stringPointer(reference.Venue),
			Year:           int32Pointer(reference.Year),
			Doi:            stringPointer(reference.DOI),
			RawText:        stringPointer(reference.RawText),
		})
		if err != nil {
			return fmt.Errorf("create paper reference: %w", err)
		}
	}
	return nil
}

func stringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func int32Pointer(value int32) *int32 {
	if value == 0 {
		return nil
	}
	return &value
}
