package jobs

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"scholarflow_server/internal/reader"
)

const cardSchemaVersion = "2.0"

type ReadSection struct {
	ID        uuid.UUID
	Label     string
	Heading   string
	Text      string
	PageStart *int32
	PageEnd   *int32
}

type ReadFigure struct {
	Label   string
	Kind    string
	Caption string
	Page    *int32
}

type ReadContext struct {
	Title    string
	Abstract string
	Sections []ReadSection
	Figures  []ReadFigure
}

type ReadRepository interface {
	UpdateJobStatus(ctx context.Context, jobID uuid.UUID, status string, errorMessage *string, attemptIncrement int32) error
	SetReadJobOutcome(ctx context.Context, jobID uuid.UUID, status string, errorMessage *string, attempt int32) error
	GetReadContext(ctx context.Context, paperID uuid.UUID) (ReadContext, error)
	SavePaperCard(ctx context.Context, paperID uuid.UUID, model, schemaVersion string, card reader.PaperCard, sectionIDByLabel map[string]uuid.UUID) error
}

type ReadPipeline struct {
	repo   ReadRepository
	reader reader.Reader
	model  string
}

func NewReadPipeline(repo ReadRepository, rdr reader.Reader, model string) *ReadPipeline {
	return &ReadPipeline{repo: repo, reader: rdr, model: model}
}

func (p *ReadPipeline) ReadPaper(ctx context.Context, payload ProcessPaperPayload, attempt int32, isFinalAttempt bool) error {
	if err := p.repo.UpdateJobStatus(ctx, payload.JobID, StatusReading, nil, 0); err != nil {
		return fmt.Errorf("mark job reading: %w", err)
	}
	err := p.read(ctx, payload)
	if err != nil {
		log.Printf("read failed paper=%s job=%s attempt=%d final=%t: %v", payload.PaperID, payload.JobID, attempt, isFinalAttempt, err)
		message := err.Error()
		status := StatusReading
		if isFinalAttempt {
			status = StatusFailed
		}
		if markErr := p.repo.SetReadJobOutcome(ctx, payload.JobID, status, &message, attempt); markErr != nil {
			return fmt.Errorf("%w; mark job outcome: %v", err, markErr)
		}
		return err
	}
	if err := p.repo.SetReadJobOutcome(ctx, payload.JobID, StatusCompleted, nil, attempt); err != nil {
		return fmt.Errorf("mark job completed: %w", err)
	}
	return nil
}

func (p *ReadPipeline) read(ctx context.Context, payload ProcessPaperPayload) error {
	rc, err := p.repo.GetReadContext(ctx, payload.PaperID)
	if err != nil {
		return fmt.Errorf("get read context: %w", err)
	}
	input := reader.Context{Title: rc.Title, Abstract: rc.Abstract}
	sectionIDByLabel := make(map[string]uuid.UUID, len(rc.Sections))
	for _, s := range rc.Sections {
		label := s.Label
		if label == "" {
			label = strconv.Itoa(len(input.Sections) + 1)
		}
		sectionIDByLabel[label] = s.ID
		input.Sections = append(input.Sections, reader.Section{
			Label:     label,
			Heading:   s.Heading,
			Text:      s.Text,
			PageStart: intFromInt32(s.PageStart),
			PageEnd:   intFromInt32(s.PageEnd),
		})
	}
	figurePageByLabel := make(map[string]*int, len(rc.Figures))
	for _, f := range rc.Figures {
		page := intFromInt32(f.Page)
		input.Figures = append(input.Figures, reader.Figure{Label: f.Label, Kind: f.Kind, Caption: f.Caption, Page: page})
		figurePageByLabel[normalizeLabel(f.Label)] = page
	}
	card, err := p.reader.ReadPaper(ctx, input)
	if err != nil {
		return fmt.Errorf("read paper: %w", err)
	}
	resolveCardPages(&card, input.Sections, figurePageByLabel)
	if err := p.repo.SavePaperCard(ctx, payload.PaperID, p.model, cardSchemaVersion, card, sectionIDByLabel); err != nil {
		return fmt.Errorf("save paper card: %w", err)
	}
	return nil
}

// resolveCardPages applies the hybrid rule: figure pages come from GROBID (by
// label), and evidence pages are clamped to their section's page range.
func resolveCardPages(card *reader.PaperCard, sections []reader.Section, figurePageByLabel map[string]*int) {
	pagesByLabel := make(map[string][2]*int, len(sections))
	for _, s := range sections {
		pagesByLabel[s.Label] = [2]*int{s.PageStart, s.PageEnd}
	}
	for i := range card.Figures {
		card.Figures[i].Page = figurePageByLabel[normalizeLabel(card.Figures[i].Label)]
	}
	for i := range card.Evidence {
		rng, ok := pagesByLabel[card.Evidence[i].SectionID]
		if !ok {
			continue
		}
		card.Evidence[i].Page = clampPage(card.Evidence[i].Page, rng[0], rng[1])
	}
}

// clampPage keeps an LLM-supplied page within [start, end]. With no section
// start it trusts the model; with no model page it falls back to start.
func clampPage(page, start, end *int) *int {
	if start == nil {
		return page
	}
	if page == nil {
		return start
	}
	v := *page
	if v < *start {
		v = *start
	}
	if end != nil && v > *end {
		v = *end
	}
	return &v
}

func normalizeLabel(label string) string {
	return strings.Join(strings.Fields(strings.ToLower(label)), " ")
}

func intFromInt32(v *int32) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}
