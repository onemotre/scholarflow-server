package jobs

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	"scholarflow_server/internal/papers"
	"scholarflow_server/internal/sources"
)

type fakeSource struct {
	name    string
	entries map[string][]sources.Entry
	err     error
}

func (f *fakeSource) Name() string { return f.name }
func (f *fakeSource) FetchRecent(ctx context.Context, category string, maxResults int) ([]sources.Entry, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.entries[category], nil
}

type fakeIngester struct {
	existing  map[string]bool
	ingested  []papers.SourceInfo
	ingestErr map[string]error
}

func (i *fakeIngester) ExistsBySourceID(ctx context.Context, sourceID string) (bool, error) {
	return i.existing[sourceID], nil
}
func (i *fakeIngester) IngestPDF(ctx context.Context, info papers.SourceInfo, body io.Reader, size int64, contentType string) (papers.UploadResult, error) {
	if err := i.ingestErr[info.SourceID]; err != nil {
		return papers.UploadResult{}, err
	}
	_, _ = io.ReadAll(body)
	i.ingested = append(i.ingested, info)
	return papers.UploadResult{PaperID: uuid.New(), JobID: uuid.New()}, nil
}

type fakeFetcher struct {
	data map[string][]byte
	err  map[string]error
}

func (f *fakeFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	if err := f.err[url]; err != nil {
		return nil, err
	}
	return f.data[url], nil
}

func sourceIDs(infos []papers.SourceInfo) []string {
	out := make([]string, len(infos))
	for i, in := range infos {
		out[i] = in.SourceID
	}
	return out
}

func TestHarvestIngestsNewDedupsKnown(t *testing.T) {
	src := &fakeSource{name: "arxiv", entries: map[string][]sources.Entry{
		"cs.CL": {
			{SourceID: "2301.00001", Title: "New", PDFURL: "u1", Published: time.Date(2023, 1, 2, 0, 0, 0, 0, time.UTC)},
			{SourceID: "2301.00002", Title: "Known", PDFURL: "u2"},
		},
	}}
	ing := &fakeIngester{existing: map[string]bool{"2301.00002": true}}
	fetch := &fakeFetcher{data: map[string][]byte{"u1": []byte("PDF1")}}
	h := NewHarvestPipeline([]sources.Source{src}, []string{"cs.CL"}, 25, ing, fetch)

	if err := h.Harvest(context.Background()); err != nil {
		t.Fatalf("Harvest error: %v", err)
	}
	got := sourceIDs(ing.ingested)
	if len(got) != 1 || got[0] != "2301.00001" {
		t.Fatalf("ingested = %v, want [2301.00001]", got)
	}
	if ing.ingested[0].SourceType != "arxiv" || ing.ingested[0].Filename != "2301.00001.pdf" {
		t.Fatalf("info = %#v", ing.ingested[0])
	}
	if ing.ingested[0].Year != 2023 {
		t.Fatalf("Year = %d, want 2023", ing.ingested[0].Year)
	}
}

func TestHarvestIsBestEffortPerEntry(t *testing.T) {
	src := &fakeSource{name: "arxiv", entries: map[string][]sources.Entry{
		"cs.CL": {
			{SourceID: "a", PDFURL: "ua"},
			{SourceID: "b", PDFURL: "ub"}, // download fails
			{SourceID: "c", PDFURL: "uc"},
		},
	}}
	ing := &fakeIngester{existing: map[string]bool{}}
	fetch := &fakeFetcher{
		data: map[string][]byte{"ua": []byte("A"), "uc": []byte("C")},
		err:  map[string]error{"ub": errors.New("404")},
	}
	h := NewHarvestPipeline([]sources.Source{src}, []string{"cs.CL"}, 25, ing, fetch)

	if err := h.Harvest(context.Background()); err != nil {
		t.Fatalf("Harvest should not fail on one bad entry: %v", err)
	}
	got := sourceIDs(ing.ingested)
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Fatalf("ingested = %v, want [a c]", got)
	}
}

func TestHarvestContinuesOnCategoryFetchError(t *testing.T) {
	src := &fakeSource{name: "arxiv", err: errors.New("arxiv 503")}
	ing := &fakeIngester{existing: map[string]bool{}}
	fetch := &fakeFetcher{}
	h := NewHarvestPipeline([]sources.Source{src}, []string{"cs.CL", "cs.AI"}, 25, ing, fetch)

	if err := h.Harvest(context.Background()); err != nil {
		t.Fatalf("Harvest should swallow fetch errors: %v", err)
	}
	if len(ing.ingested) != 0 {
		t.Fatalf("ingested = %v, want none", sourceIDs(ing.ingested))
	}
}
