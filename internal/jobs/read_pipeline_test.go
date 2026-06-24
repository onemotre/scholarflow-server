package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"scholarflow_server/internal/reader"
)

type recordedOutcome struct {
	status  string
	errMsg  string
	attempt int32
}

type fakeReadRepo struct {
	statuses []string
	rc       ReadContext
	saved    reader.PaperCard
	savedMap map[string]uuid.UUID
	failSave error
	outcomes []recordedOutcome
}

func (r *fakeReadRepo) UpdateJobStatus(ctx context.Context, jobID uuid.UUID, status string, errorMessage *string, attemptIncrement int32) error {
	r.statuses = append(r.statuses, status)
	return nil
}

func (r *fakeReadRepo) SetReadJobOutcome(ctx context.Context, jobID uuid.UUID, status string, errorMessage *string, attempt int32) error {
	msg := ""
	if errorMessage != nil {
		msg = *errorMessage
	}
	r.statuses = append(r.statuses, status)
	r.outcomes = append(r.outcomes, recordedOutcome{status: status, errMsg: msg, attempt: attempt})
	return nil
}

func (r *fakeReadRepo) GetReadContext(ctx context.Context, paperID uuid.UUID) (ReadContext, error) {
	return r.rc, nil
}

func (r *fakeReadRepo) SavePaperCard(ctx context.Context, paperID uuid.UUID, model, schemaVersion string, card reader.PaperCard, sectionIDByLabel map[string]uuid.UUID) error {
	if r.failSave != nil {
		return r.failSave
	}
	r.saved = card
	r.savedMap = sectionIDByLabel
	return nil
}

type fakeReader struct {
	card reader.PaperCard
	err  error
	got  reader.Context
}

func (f *fakeReader) ReadPaper(ctx context.Context, input reader.Context) (reader.PaperCard, error) {
	f.got = input
	return f.card, f.err
}

func TestReadPipelineCompletes(t *testing.T) {
	sectionID := uuid.New()
	repo := &fakeReadRepo{rc: ReadContext{
		Title:    "T",
		Abstract: "A",
		Sections: []ReadSection{{ID: sectionID, Label: "1", Heading: "Intro", Text: "Body"}},
		Figures:  []ReadFigure{{Label: "Figure 1", Kind: "figure", Caption: "cap"}},
	}}
	rdr := &fakeReader{card: reader.PaperCard{Background: "bg", Problem: "p", Method: "m", Implementation: "impl"}}
	pipe := NewReadPipeline(repo, rdr, "gpt-4o-mini")

	err := pipe.ReadPaper(context.Background(), ProcessPaperPayload{PaperID: uuid.New(), JobID: uuid.New()}, 1, true)
	if err != nil {
		t.Fatalf("ReadPaper error: %v", err)
	}
	if got := strings.Join(repo.statuses, ","); got != "reading,completed" {
		t.Fatalf("statuses = %s", got)
	}
	if repo.saved.Method != "m" {
		t.Fatalf("saved method = %q", repo.saved.Method)
	}
	if repo.savedMap["1"] != sectionID {
		t.Fatalf("label map = %#v", repo.savedMap)
	}
	if rdr.got.Sections[0].Label != "1" {
		t.Fatalf("reader did not receive labeled section: %#v", rdr.got.Sections)
	}
	if len(rdr.got.Figures) != 1 || rdr.got.Figures[0].Caption != "cap" {
		t.Fatalf("reader did not receive figures: %#v", rdr.got.Figures)
	}
}

func TestReadPipelineResolvesPages(t *testing.T) {
	sectionID := uuid.New()
	p3, p5, p9 := int32(3), int32(5), int32(9)
	repo := &fakeReadRepo{rc: ReadContext{
		Title:    "T",
		Abstract: "A",
		Sections: []ReadSection{{ID: sectionID, Label: "1", Heading: "Results", Text: "Body", PageStart: &p3, PageEnd: &p5}},
		Figures:  []ReadFigure{{Label: "Figure 2", Kind: "figure", Caption: "cap", Page: &p9}},
	}}
	ev0 := 7 // out of [3,5] -> clamp to 5
	ev1 := 4 // within range -> keep
	rdr := &fakeReader{card: reader.PaperCard{
		Background: "bg", Problem: "p", Method: "m", Implementation: "impl",
		Results: []string{"r0"},
		Figures: []reader.FigureRef{{Label: "figure 2", ClaimKey: "results", ClaimIndex: intPtr(0)}},
		Evidence: []reader.Evidence{
			{ClaimKey: "results", SectionID: "1", Page: &ev0},
			{ClaimKey: "results", SectionID: "1", Page: &ev1},
			{ClaimKey: "method", SectionID: "999"}, // unknown section -> untouched
		},
	}}
	pipe := NewReadPipeline(repo, rdr, "m")

	if err := pipe.ReadPaper(context.Background(), ProcessPaperPayload{PaperID: uuid.New(), JobID: uuid.New()}, 1, true); err != nil {
		t.Fatalf("ReadPaper error: %v", err)
	}
	// Figure page comes from GROBID (label match is case/space-insensitive).
	if got := repo.saved.Figures[0].Page; got == nil || *got != 9 {
		t.Fatalf("figure page = %v, want 9", got)
	}
	if got := repo.saved.Evidence[0].Page; got == nil || *got != 5 {
		t.Fatalf("evidence[0] page = %v, want clamped 5", got)
	}
	if got := repo.saved.Evidence[1].Page; got == nil || *got != 4 {
		t.Fatalf("evidence[1] page = %v, want 4", got)
	}
	if repo.saved.Evidence[2].Page != nil {
		t.Fatalf("evidence[2] page = %v, want nil (unknown section)", repo.saved.Evidence[2].Page)
	}
	// Reader received page context.
	if rdr.got.Sections[0].PageStart == nil || *rdr.got.Sections[0].PageStart != 3 {
		t.Fatalf("reader section page = %v", rdr.got.Sections[0].PageStart)
	}
}

func intPtr(v int) *int { return &v }

func TestReadPipelineNonFinalFailureStaysReading(t *testing.T) {
	repo := &fakeReadRepo{rc: ReadContext{Title: "T"}}
	rdr := &fakeReader{err: errors.New("llm down")}
	pipe := NewReadPipeline(repo, rdr, "gpt-4o-mini")

	err := pipe.ReadPaper(context.Background(), ProcessPaperPayload{PaperID: uuid.New(), JobID: uuid.New()}, 1, false)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	last := repo.outcomes[len(repo.outcomes)-1]
	if last.status != StatusReading {
		t.Fatalf("non-final failure status = %q, want reading", last.status)
	}
	if last.attempt != 1 || last.errMsg == "" {
		t.Fatalf("outcome = %#v, want attempt 1 with error", last)
	}
}

func TestReadPipelineFinalFailureMarksFailed(t *testing.T) {
	repo := &fakeReadRepo{rc: ReadContext{Title: "T"}}
	rdr := &fakeReader{err: errors.New("llm down")}
	pipe := NewReadPipeline(repo, rdr, "gpt-4o-mini")

	err := pipe.ReadPaper(context.Background(), ProcessPaperPayload{PaperID: uuid.New(), JobID: uuid.New()}, 4, true)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	last := repo.outcomes[len(repo.outcomes)-1]
	if last.status != StatusFailed {
		t.Fatalf("final failure status = %q, want failed", last.status)
	}
	if last.attempt != 4 {
		t.Fatalf("final attempt = %d, want 4", last.attempt)
	}
}
