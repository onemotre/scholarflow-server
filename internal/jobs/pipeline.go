package jobs

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"scholarflow_server/internal/parser"
	"scholarflow_server/internal/storage"
)

const (
	StatusProcessing = "processing"
	StatusParsed     = "parsed"
	StatusReading    = "reading"
	StatusCompleted  = "completed"
	StatusFailed     = "failed"
)

type PipelineRepository interface {
	UpdateJobStatus(ctx context.Context, jobID uuid.UUID, status string, errorMessage *string, attemptIncrement int32) error
	GetPaperPDFAsset(ctx context.Context, paperID uuid.UUID) (storage.Object, error)
	CreateTEIAsset(ctx context.Context, paperID uuid.UUID, asset storage.Object) error
	SaveParsedPaper(ctx context.Context, paperID uuid.UUID, parsed parser.ParsedPaper) error
}

type Pipeline struct {
	repo   PipelineRepository
	store  storage.Store
	parser parser.Parser
}

func NewPipeline(repo PipelineRepository, store storage.Store, parser parser.Parser) *Pipeline {
	return &Pipeline{repo: repo, store: store, parser: parser}
}

func (p *Pipeline) ProcessPaper(ctx context.Context, payload ProcessPaperPayload) error {
	if err := p.repo.UpdateJobStatus(ctx, payload.JobID, StatusProcessing, nil, 0); err != nil {
		return fmt.Errorf("mark job processing: %w", err)
	}
	err := p.process(ctx, payload)
	if err != nil {
		message := err.Error()
		if markErr := p.repo.UpdateJobStatus(ctx, payload.JobID, StatusFailed, &message, 1); markErr != nil {
			return fmt.Errorf("%w; mark job failed: %v", err, markErr)
		}
		return err
	}
	if err := p.repo.UpdateJobStatus(ctx, payload.JobID, StatusParsed, nil, 0); err != nil {
		return fmt.Errorf("mark job parsed: %w", err)
	}
	return nil
}

func (p *Pipeline) process(ctx context.Context, payload ProcessPaperPayload) error {
	pdfAsset, err := p.repo.GetPaperPDFAsset(ctx, payload.PaperID)
	if err != nil {
		return fmt.Errorf("get paper pdf asset: %w", err)
	}
	pdf, err := p.store.Get(ctx, pdfAsset.Key)
	if err != nil {
		return fmt.Errorf("read pdf object: %w", err)
	}
	defer pdf.Close()
	parsed, err := p.parser.ParsePDF(ctx, pdfAsset.Key, pdf)
	if err != nil {
		return fmt.Errorf("parse pdf: %w", err)
	}
	tei := strings.NewReader(parsed.RawTEI)
	teiKey := fmt.Sprintf("papers/%s/grobid.tei.xml", payload.PaperID.String())
	teiAsset, err := p.store.Put(ctx, teiKey, tei, int64(len(parsed.RawTEI)), "application/xml")
	if err != nil {
		return fmt.Errorf("store grobid tei: %w", err)
	}
	if err := p.repo.CreateTEIAsset(ctx, payload.PaperID, teiAsset); err != nil {
		return fmt.Errorf("create tei asset: %w", err)
	}
	if err := p.repo.SaveParsedPaper(ctx, payload.PaperID, parsed); err != nil {
		return fmt.Errorf("save parsed paper: %w", err)
	}
	return nil
}
