package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"scholarflow_server/internal/db"
	"scholarflow_server/internal/parser"
	"scholarflow_server/internal/reader"
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

func (r *SQLRepository) SetReadJobOutcome(ctx context.Context, jobID uuid.UUID, status string, errorMessage *string, attempt int32) error {
	_, err := r.queries.SetJobStatusAndAttempt(ctx, db.SetJobStatusAndAttemptParams{
		ID:           jobID,
		Status:       status,
		ErrorMessage: errorMessage,
		AttemptCount: attempt,
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
	if err := r.queries.DeletePaperFiguresByPaper(ctx, paperID); err != nil {
		return fmt.Errorf("delete paper figures: %w", err)
	}
	for _, fig := range parsed.Figures {
		_, err := r.queries.CreatePaperFigure(ctx, db.CreatePaperFigureParams{
			PaperID:      paperID,
			Kind:         fig.Kind,
			Label:        fig.Label,
			Caption:      fig.Caption,
			FigureOrder:  fig.Order,
			Page:         fig.Page,
			ImageAssetID: pgtype.UUID{},
		})
		if err != nil {
			return fmt.Errorf("create paper figure: %w", err)
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

func (r *SQLRepository) GetReadContext(ctx context.Context, paperID uuid.UUID) (ReadContext, error) {
	paper, err := r.queries.GetPaper(ctx, paperID)
	if err != nil {
		return ReadContext{}, fmt.Errorf("get paper: %w", err)
	}
	sections, err := r.queries.ListPaperSections(ctx, paperID)
	if err != nil {
		return ReadContext{}, fmt.Errorf("list sections: %w", err)
	}
	rc := ReadContext{Title: stringValue(paper.Title), Abstract: stringValue(paper.Abstract)}
	for _, s := range sections {
		rc.Sections = append(rc.Sections, ReadSection{
			ID:      s.ID,
			Label:   strconv.FormatInt(int64(s.SectionOrder), 10),
			Heading: stringValue(s.Heading),
			Text:    s.Text,
		})
	}
	figures, err := r.queries.ListPaperFigures(ctx, paperID)
	if err != nil {
		return ReadContext{}, fmt.Errorf("list figures: %w", err)
	}
	for _, f := range figures {
		rc.Figures = append(rc.Figures, ReadFigure{Label: f.Label, Kind: f.Kind, Caption: f.Caption})
	}
	return rc, nil
}

func (r *SQLRepository) SavePaperCard(ctx context.Context, paperID uuid.UUID, model, schemaVersion string, card reader.PaperCard, sectionIDByLabel map[string]uuid.UUID) error {
	content, err := json.Marshal(card)
	if err != nil {
		return fmt.Errorf("marshal card: %w", err)
	}
	if err := r.queries.DeletePaperCardsByPaper(ctx, paperID); err != nil {
		return fmt.Errorf("delete existing cards: %w", err)
	}
	created, err := r.queries.CreatePaperCard(ctx, db.CreatePaperCardParams{
		PaperID:       paperID,
		SchemaVersion: schemaVersion,
		Model:         model,
		ContentJson:   content,
	})
	if err != nil {
		return fmt.Errorf("create card: %w", err)
	}
	cardID := pgtype.UUID{Bytes: created.ID, Valid: true}
	for _, ev := range card.Evidence {
		sectionID := pgtype.UUID{}
		if id, ok := sectionIDByLabel[ev.SectionID]; ok {
			sectionID = pgtype.UUID{Bytes: id, Valid: true}
		}
		_, err := r.queries.CreatePaperEvidence(ctx, db.CreatePaperEvidenceParams{
			PaperID:      paperID,
			PaperCardID:  cardID,
			ClaimKey:     ev.ClaimKey,
			EvidenceType: ev.EvidenceType,
			SectionID:    sectionID,
			AssetID:      pgtype.UUID{},
			Page:         intPointer(ev.Page),
			Locator:      stringPointer(ev.Locator),
			Snippet:      stringPointer(ev.Snippet),
			Confidence:   ev.Confidence,
		})
		if err != nil {
			return fmt.Errorf("create evidence: %w", err)
		}
	}
	return nil
}

func intPointer(value *int) *int32 {
	if value == nil {
		return nil
	}
	v := int32(*value)
	return &v
}
