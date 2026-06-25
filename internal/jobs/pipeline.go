package jobs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/google/uuid"

	"scholarflow_server/internal/figures"
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

// Minimum figure bounding-box size in PDF points. GROBID sometimes reports only
// a figure's caption-label sliver (e.g. ~10x3 pt) instead of the graphic; cropping
// those yields a few-pixel garbage image, so we skip them and leave the figure
// without an extracted image rather than serve a broken crop.
const (
	minFigureWidthPt  = 40.0
	minFigureHeightPt = 30.0
)

type PipelineRepository interface {
	UpdateJobStatus(ctx context.Context, jobID uuid.UUID, status string, errorMessage *string, attemptIncrement int32) error
	GetPaperPDFAsset(ctx context.Context, paperID uuid.UUID) (storage.Object, error)
	CreateTEIAsset(ctx context.Context, paperID uuid.UUID, asset storage.Object) error
	SaveParsedPaper(ctx context.Context, paperID uuid.UUID, parsed parser.ParsedPaper) error
	AttachFigureImage(ctx context.Context, paperID uuid.UUID, figureOrder int32, asset storage.Object) error
	ClearFigureImages(ctx context.Context, paperID uuid.UUID) error
}

type ReadEnqueuer interface {
	EnqueuePaperRead(ctx context.Context, paperID, jobID uuid.UUID) (string, error)
}

type Pipeline struct {
	repo         PipelineRepository
	store        storage.Store
	parser       parser.Parser
	readEnqueuer ReadEnqueuer
	cropper      figures.Cropper
	figDPI       int
}

func NewPipeline(repo PipelineRepository, store storage.Store, parser parser.Parser, readEnqueuer ReadEnqueuer, cropper figures.Cropper, figDPI int) *Pipeline {
	return &Pipeline{repo: repo, store: store, parser: parser, readEnqueuer: readEnqueuer, cropper: cropper, figDPI: figDPI}
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
	if p.readEnqueuer != nil {
		if _, err := p.readEnqueuer.EnqueuePaperRead(ctx, payload.PaperID, payload.JobID); err != nil {
			return fmt.Errorf("enqueue paper read: %w", err)
		}
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
	p.extractFigures(ctx, payload.PaperID, pdfAsset.Key, parsed.Figures)
	return nil
}

// extractFigures crops each figure with a bounding box out of the PDF and links
// the resulting image. It is best-effort: every failure is logged and skipped so
// the parse job is never affected.
func (p *Pipeline) extractFigures(ctx context.Context, paperID uuid.UUID, pdfKey string, figs []parser.Figure) {
	if p.cropper == nil {
		return
	}
	if err := p.repo.ClearFigureImages(ctx, paperID); err != nil {
		log.Printf("figure extract: clear prior images paper=%s: %v", paperID, err)
	}
	hasBox := false
	for _, f := range figs {
		if f.BBox != nil {
			hasBox = true
			break
		}
	}
	if !hasBox {
		return
	}
	// The original PDF reader was consumed by ParsePDF, so re-fetch it and write
	// it to a temp file that pdftoppm can open.
	rc, err := p.store.Get(ctx, pdfKey)
	if err != nil {
		log.Printf("figure extract: get pdf paper=%s: %v", paperID, err)
		return
	}
	defer rc.Close()
	tmp, err := os.CreateTemp("", "scholarflow-*.pdf")
	if err != nil {
		log.Printf("figure extract: temp file paper=%s: %v", paperID, err)
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		log.Printf("figure extract: copy pdf paper=%s: %v", paperID, err)
		return
	}
	tmp.Close()

	for _, f := range figs {
		if f.BBox == nil {
			continue
		}
		if f.BBox.W < minFigureWidthPt || f.BBox.H < minFigureHeightPt {
			log.Printf("figure extract: skip degenerate bbox paper=%s order=%d (%.1fx%.1f pt)", paperID, f.Order, f.BBox.W, f.BBox.H)
			continue
		}
		img, err := p.cropper.Crop(ctx, tmp.Name(), int(f.BBox.Page),
			figures.Rect{X: f.BBox.X, Y: f.BBox.Y, W: f.BBox.W, H: f.BBox.H}, p.figDPI)
		if err != nil {
			log.Printf("figure extract: crop paper=%s order=%d: %v", paperID, f.Order, err)
			continue
		}
		key := fmt.Sprintf("papers/%s/figures/%d.png", paperID.String(), f.Order)
		obj, err := p.store.Put(ctx, key, bytes.NewReader(img), int64(len(img)), "image/png")
		if err != nil {
			log.Printf("figure extract: put paper=%s order=%d: %v", paperID, f.Order, err)
			continue
		}
		if err := p.repo.AttachFigureImage(ctx, paperID, f.Order, obj); err != nil {
			log.Printf("figure extract: attach paper=%s order=%d: %v", paperID, f.Order, err)
			continue
		}
	}
}
