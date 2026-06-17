package jobs

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"scholarflow_server/internal/parser"
	"scholarflow_server/internal/storage"
)

type fakePipelineRepo struct {
	statuses       []string
	pdfAsset       storage.Object
	teiAsset       storage.Object
	saved          parser.ParsedPaper
	failPDFAsset   error
	failSaveParsed error
	failStatus     error
}

func (r *fakePipelineRepo) UpdateJobStatus(ctx context.Context, jobID uuid.UUID, status string, errorMessage *string, attemptIncrement int32) error {
	if r.failStatus != nil {
		return r.failStatus
	}
	r.statuses = append(r.statuses, status)
	return nil
}

func (r *fakePipelineRepo) GetPaperPDFAsset(ctx context.Context, paperID uuid.UUID) (storage.Object, error) {
	if r.failPDFAsset != nil {
		return storage.Object{}, r.failPDFAsset
	}
	return r.pdfAsset, nil
}

func (r *fakePipelineRepo) CreateTEIAsset(ctx context.Context, paperID uuid.UUID, asset storage.Object) error {
	r.teiAsset = asset
	return nil
}

func (r *fakePipelineRepo) SaveParsedPaper(ctx context.Context, paperID uuid.UUID, parsed parser.ParsedPaper) error {
	if r.failSaveParsed != nil {
		return r.failSaveParsed
	}
	r.saved = parsed
	return nil
}

type fakePipelineStore struct {
	getKey  string
	putKey  string
	putBody string
	pdf     string
}

func (s *fakePipelineStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	s.getKey = key
	return io.NopCloser(strings.NewReader(s.pdf)), nil
}

func (s *fakePipelineStore) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) (storage.Object, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return storage.Object{}, err
	}
	s.putKey = key
	s.putBody = string(data)
	return storage.Object{
		Bucket:      "papers",
		Key:         key,
		ContentType: contentType,
		SizeBytes:   size,
		Checksum:    "checksum",
	}, nil
}

type fakeParser struct {
	filename string
	body     string
	err      error
	parsed   parser.ParsedPaper
}

func (p *fakeParser) ParsePDF(ctx context.Context, filename string, body io.Reader) (parser.ParsedPaper, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return parser.ParsedPaper{}, err
	}
	p.filename = filename
	p.body = string(data)
	if p.err != nil {
		return parser.ParsedPaper{}, p.err
	}
	return p.parsed, nil
}

func TestPipelineProcessesPaperToParsed(t *testing.T) {
	paperID := uuid.New()
	jobID := uuid.New()
	repo := &fakePipelineRepo{
		pdfAsset: storage.Object{Bucket: "papers", Key: "papers/input.pdf", ContentType: "application/pdf", SizeBytes: 7},
	}
	store := &fakePipelineStore{pdf: "pdfdata"}
	parserFake := &fakeParser{parsed: parser.ParsedPaper{
		Title:      "Parsed title",
		Abstract:   "Parsed abstract",
		DOI:        "10.123/example",
		Year:       2026,
		RawTEI:     "<TEI/>",
		Authors:    []parser.Author{{Order: 1, DisplayName: "Ada Lovelace", ORCID: "0000"}},
		Sections:   []parser.Section{{Order: 1, Heading: "Intro", Text: "Body"}},
		References: []parser.Reference{{Order: 1, Title: "Reference", Authors: []string{"Grace Hopper"}, Year: 1952}},
	}}
	service := NewPipeline(repo, store, parserFake)

	err := service.ProcessPaper(context.Background(), ProcessPaperPayload{PaperID: paperID, JobID: jobID})
	if err != nil {
		t.Fatalf("ProcessPaper returned error: %v", err)
	}
	if got := strings.Join(repo.statuses, ","); got != "processing,parsed" {
		t.Fatalf("statuses = %s", got)
	}
	if store.getKey != "papers/input.pdf" {
		t.Fatalf("Get key = %q", store.getKey)
	}
	if parserFake.body != "pdfdata" {
		t.Fatalf("parser body = %q", parserFake.body)
	}
	if store.putBody != "<TEI/>" {
		t.Fatalf("TEI body = %q", store.putBody)
	}
	if repo.saved.Title != "Parsed title" {
		t.Fatalf("saved title = %q", repo.saved.Title)
	}
}

func TestPipelineMarksJobFailedWhenParserFails(t *testing.T) {
	paperID := uuid.New()
	jobID := uuid.New()
	repo := &fakePipelineRepo{
		pdfAsset: storage.Object{Bucket: "papers", Key: "papers/input.pdf", ContentType: "application/pdf", SizeBytes: 7},
	}
	store := &fakePipelineStore{pdf: "pdfdata"}
	parserFake := &fakeParser{err: errors.New("grobid unavailable")}
	service := NewPipeline(repo, store, parserFake)

	err := service.ProcessPaper(context.Background(), ProcessPaperPayload{PaperID: paperID, JobID: jobID})
	if err == nil {
		t.Fatal("ProcessPaper returned nil, want error")
	}
	if got := strings.Join(repo.statuses, ","); got != "processing,failed" {
		t.Fatalf("statuses = %s", got)
	}
}

func TestProcessorRejectsMalformedPayload(t *testing.T) {
	processor := NewProcessor(nil)
	err := processor.HandleProcessPaper(context.Background(), asynq.NewTask(TypeProcessPaper, []byte("{bad json")))
	if err == nil {
		t.Fatal("HandleProcessPaper returned nil, want error")
	}
}

func TestReadProcessorRejectsMalformedPayload(t *testing.T) {
	processor := NewReadProcessor(nil)
	err := processor.HandleReadPaper(context.Background(), asynq.NewTask(TypeReadPaper, []byte("{bad json")))
	if err == nil {
		t.Fatal("HandleReadPaper returned nil, want error")
	}
}
