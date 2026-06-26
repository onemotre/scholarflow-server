package papers

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"

	"scholarflow_server/internal/storage"
)

type fakeRepo struct {
	created   SourceInfo
	exists    bool
	taskIDSet string
}

func (r *fakeRepo) CreatePaperUpload(ctx context.Context, info SourceInfo, asset storage.Object) (UploadResult, error) {
	r.created = info
	return UploadResult{PaperID: uuid.New(), JobID: uuid.New()}, nil
}
func (r *fakeRepo) SetJobTaskID(ctx context.Context, jobID uuid.UUID, taskID string) error {
	r.taskIDSet = taskID
	return nil
}
func (r *fakeRepo) GetPaperBySourceID(ctx context.Context, sourceID string) (bool, error) {
	return r.exists, nil
}

type fakeStore struct{ putKey, putBody string }

func (s *fakeStore) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) (storage.Object, error) {
	data, _ := io.ReadAll(body)
	s.putKey = key
	s.putBody = string(data)
	return storage.Object{Bucket: "papers", Key: key, ContentType: contentType, SizeBytes: size}, nil
}
func (s *fakeStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

type fakeEnqueuer struct{ calls int }

func (e *fakeEnqueuer) EnqueuePaperProcessing(ctx context.Context, paperID, jobID uuid.UUID) (string, error) {
	e.calls++
	return "task-xyz", nil
}

func TestUploadPDFUsesLocalSource(t *testing.T) {
	repo := &fakeRepo{}
	store := &fakeStore{}
	enq := &fakeEnqueuer{}
	svc := NewService(repo, store, enq)

	_, err := svc.UploadPDF(context.Background(), "paper.pdf", strings.NewReader("PDF"), 3, "application/pdf")
	if err != nil {
		t.Fatalf("UploadPDF error: %v", err)
	}
	if repo.created.SourceType != "local_pdf" {
		t.Fatalf("SourceType = %q, want local_pdf", repo.created.SourceType)
	}
	if repo.created.SourceID != "" {
		t.Fatalf("SourceID = %q, want empty", repo.created.SourceID)
	}
	if enq.calls != 1 || repo.taskIDSet != "task-xyz" {
		t.Fatalf("enqueue calls=%d taskID=%q", enq.calls, repo.taskIDSet)
	}
}

func TestUploadPDFRejectsNonPDF(t *testing.T) {
	svc := NewService(&fakeRepo{}, &fakeStore{}, &fakeEnqueuer{})
	if _, err := svc.UploadPDF(context.Background(), "x.txt", strings.NewReader("x"), 1, "text/plain"); err == nil {
		t.Fatal("want error for non-pdf content type")
	}
}

func TestIngestPDFCarriesArxivMetadata(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, &fakeStore{}, &fakeEnqueuer{})
	info := SourceInfo{SourceType: "arxiv", SourceID: "2301.00001", Filename: "2301.00001.pdf", Title: "T", Year: 2023}

	_, err := svc.IngestPDF(context.Background(), info, strings.NewReader("PDF"), 3, "application/pdf")
	if err != nil {
		t.Fatalf("IngestPDF error: %v", err)
	}
	if repo.created.SourceID != "2301.00001" || repo.created.Title != "T" || repo.created.Year != 2023 {
		t.Fatalf("created = %#v", repo.created)
	}
}

func TestExistsBySourceID(t *testing.T) {
	repo := &fakeRepo{exists: true}
	svc := NewService(repo, &fakeStore{}, &fakeEnqueuer{})
	ok, err := svc.ExistsBySourceID(context.Background(), "2301.00001")
	if err != nil || !ok {
		t.Fatalf("ExistsBySourceID = %v, %v; want true, nil", ok, err)
	}
}
