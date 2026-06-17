package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"scholarflow_server/internal/reader"
)

type fakeReadRepo struct {
	statuses []string
	rc       ReadContext
	saved    reader.PaperCard
	savedMap map[string]uuid.UUID
	failSave error
}

func (r *fakeReadRepo) UpdateJobStatus(ctx context.Context, jobID uuid.UUID, status string, errorMessage *string, attemptIncrement int32) error {
	r.statuses = append(r.statuses, status)
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
	}}
	rdr := &fakeReader{card: reader.PaperCard{Background: "bg", Problem: "p", Method: "m", Implementation: "impl"}}
	pipe := NewReadPipeline(repo, rdr, "gpt-4o-mini")

	err := pipe.ReadPaper(context.Background(), ProcessPaperPayload{PaperID: uuid.New(), JobID: uuid.New()})
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
}

func TestReadPipelineFailsOnReaderError(t *testing.T) {
	repo := &fakeReadRepo{rc: ReadContext{Title: "T"}}
	rdr := &fakeReader{err: errors.New("llm down")}
	pipe := NewReadPipeline(repo, rdr, "gpt-4o-mini")

	err := pipe.ReadPaper(context.Background(), ProcessPaperPayload{PaperID: uuid.New(), JobID: uuid.New()})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if got := strings.Join(repo.statuses, ","); got != "reading,failed" {
		t.Fatalf("statuses = %s", got)
	}
}
