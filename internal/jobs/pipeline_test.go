package jobs

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"scholarflow_server/internal/figures"
	"scholarflow_server/internal/parser"
	"scholarflow_server/internal/storage"
)

type figureAttach struct {
	order int32
	asset storage.Object
}

type fakePipelineRepo struct {
	statuses            []string
	pdfAsset            storage.Object
	teiAsset            storage.Object
	saved               parser.ParsedPaper
	failPDFAsset        error
	failSaveParsed      error
	failStatus          error
	attached            []figureAttach
	failAttach          error
	clearedFigureImages []uuid.UUID
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

func (r *fakePipelineRepo) AttachFigureImage(ctx context.Context, paperID uuid.UUID, figureOrder int32, asset storage.Object) error {
	if r.failAttach != nil {
		return r.failAttach
	}
	r.attached = append(r.attached, figureAttach{order: figureOrder, asset: asset})
	return nil
}

func (r *fakePipelineRepo) ClearFigureImages(ctx context.Context, paperID uuid.UUID) error {
	r.clearedFigureImages = append(r.clearedFigureImages, paperID)
	return nil
}

type fakePipelineStore struct {
	getKey  string
	putKey  string
	putBody string
	pdf     string
	puts    []struct{ Key, Body string }
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
	s.puts = append(s.puts, struct{ Key, Body string }{key, string(data)})
	return storage.Object{
		Bucket:      "papers",
		Key:         key,
		ContentType: contentType,
		SizeBytes:   size,
		Checksum:    "checksum",
	}, nil
}

func (s *fakePipelineStore) Delete(ctx context.Context, key string) error {
	return nil
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

type fakeCropper struct {
	calls int
	err   error
}

func (c *fakeCropper) Crop(ctx context.Context, pdfPath string, page int, rect figures.Rect, dpi int) ([]byte, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return []byte("PNGDATA"), nil
}

func TestPipelineExtractsFigures(t *testing.T) {
	paperID := uuid.New()
	box := parser.FigureBox{Page: 2, X: 10, Y: 20, W: 200, H: 150}
	repo := &fakePipelineRepo{pdfAsset: storage.Object{Key: "papers/input.pdf"}}
	store := &fakePipelineStore{pdf: "pdfdata"}
	parserFake := &fakeParser{parsed: parser.ParsedPaper{
		Title:  "T",
		RawTEI: "<TEI/>",
		Figures: []parser.Figure{
			{Order: 1, Kind: "figure", Label: "Figure 1", BBox: &box},
			{Order: 2, Kind: "figure", Label: "Figure 2"}, // no BBox -> skipped
		},
	}}
	cropper := &fakeCropper{}
	service := NewPipeline(repo, store, parserFake, nil, cropper, 150)

	if err := service.ProcessPaper(context.Background(), ProcessPaperPayload{PaperID: paperID, JobID: uuid.New()}); err != nil {
		t.Fatalf("ProcessPaper error: %v", err)
	}
	if cropper.calls != 1 {
		t.Fatalf("cropper calls = %d, want 1", cropper.calls)
	}
	if len(repo.attached) != 1 || repo.attached[0].order != 1 {
		t.Fatalf("attached = %#v, want one entry for order 1", repo.attached)
	}
	wantKey := "papers/" + paperID.String() + "/figures/1.png"
	found := false
	for _, p := range store.puts {
		if p.Key == wantKey && p.Body == "PNGDATA" {
			found = true
		}
	}
	if !found {
		t.Fatalf("figure PNG not stored at %q; puts=%#v", wantKey, store.puts)
	}
	if got := strings.Join(repo.statuses, ","); got != "processing,parsed" {
		t.Fatalf("statuses = %s", got)
	}
	if len(repo.clearedFigureImages) != 1 || repo.clearedFigureImages[0] != paperID {
		t.Fatalf("ClearFigureImages calls = %v, want exactly one call with paperID %s", repo.clearedFigureImages, paperID)
	}
}

func TestPipelineSkipsDegenerateFigureBBox(t *testing.T) {
	// GROBID sometimes reports only a caption-label sliver; cropping it yields a
	// few-pixel garbage image, so the pipeline must skip it (no crop, no attach).
	box := parser.FigureBox{Page: 1, X: 10, Y: 10, W: 12, H: 4}
	repo := &fakePipelineRepo{pdfAsset: storage.Object{Key: "papers/input.pdf"}}
	store := &fakePipelineStore{pdf: "pdfdata"}
	parserFake := &fakeParser{parsed: parser.ParsedPaper{
		Title:   "T",
		RawTEI:  "<TEI/>",
		Figures: []parser.Figure{{Order: 1, Kind: "figure", Label: "Figure 1", BBox: &box}},
	}}
	cropper := &fakeCropper{}
	service := NewPipeline(repo, store, parserFake, nil, cropper, 150)

	if err := service.ProcessPaper(context.Background(), ProcessPaperPayload{PaperID: uuid.New(), JobID: uuid.New()}); err != nil {
		t.Fatalf("ProcessPaper error: %v", err)
	}
	if cropper.calls != 0 {
		t.Fatalf("cropper calls = %d, want 0 (degenerate bbox skipped)", cropper.calls)
	}
	if len(repo.attached) != 0 {
		t.Fatalf("attached = %#v, want none", repo.attached)
	}
}

func TestPipelineFigureExtractIsBestEffort(t *testing.T) {
	box := parser.FigureBox{Page: 1, X: 0, Y: 0, W: 200, H: 150}
	repo := &fakePipelineRepo{pdfAsset: storage.Object{Key: "papers/input.pdf"}}
	store := &fakePipelineStore{pdf: "pdfdata"}
	parserFake := &fakeParser{parsed: parser.ParsedPaper{
		Title:   "T",
		RawTEI:  "<TEI/>",
		Figures: []parser.Figure{{Order: 1, BBox: &box}},
	}}
	cropper := &fakeCropper{err: errors.New("pdftoppm boom")}
	service := NewPipeline(repo, store, parserFake, nil, cropper, 150)

	if err := service.ProcessPaper(context.Background(), ProcessPaperPayload{PaperID: uuid.New(), JobID: uuid.New()}); err != nil {
		t.Fatalf("ProcessPaper should not fail on crop error: %v", err)
	}
	if len(repo.attached) != 0 {
		t.Fatalf("attached = %#v, want none", repo.attached)
	}
	if got := strings.Join(repo.statuses, ","); got != "processing,parsed" {
		t.Fatalf("statuses = %s", got)
	}
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
	service := NewPipeline(repo, store, parserFake, nil, nil, 0)

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
	service := NewPipeline(repo, store, parserFake, nil, nil, 0)

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

type fakeReadEnqueuer struct {
	calls int
}

func (e *fakeReadEnqueuer) EnqueuePaperRead(ctx context.Context, paperID, jobID uuid.UUID) (string, error) {
	e.calls++
	return "task-1", nil
}

func TestPipelineEnqueuesReadAfterParse(t *testing.T) {
	repo := &fakePipelineRepo{pdfAsset: storage.Object{Key: "papers/input.pdf"}}
	store := &fakePipelineStore{pdf: "pdfdata"}
	parserFake := &fakeParser{parsed: parser.ParsedPaper{Title: "T", RawTEI: "<TEI/>"}}
	enq := &fakeReadEnqueuer{}
	service := NewPipeline(repo, store, parserFake, enq, nil, 0)

	err := service.ProcessPaper(context.Background(), ProcessPaperPayload{PaperID: uuid.New(), JobID: uuid.New()})
	if err != nil {
		t.Fatalf("ProcessPaper error: %v", err)
	}
	if enq.calls != 1 {
		t.Fatalf("read enqueue calls = %d, want 1", enq.calls)
	}
}
