package papers

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type fakeAdminRepo struct {
	assets       []AssetRef
	assetsErr    error
	deleteRows   int64
	deleteErr    error
	job          JobStatus
	jobErr       error
	requeueRows  int64
	sectionCount int64
	taskIDSet    string
}

func (r *fakeAdminRepo) ListPaperAssets(ctx context.Context, paperID uuid.UUID) ([]AssetRef, error) {
	return r.assets, r.assetsErr
}
func (r *fakeAdminRepo) DeletePaper(ctx context.Context, paperID uuid.UUID) (int64, error) {
	return r.deleteRows, r.deleteErr
}
func (r *fakeAdminRepo) GetLatestJobByPaper(ctx context.Context, paperID uuid.UUID) (JobStatus, error) {
	return r.job, r.jobErr
}
func (r *fakeAdminRepo) RequeueJob(ctx context.Context, jobID uuid.UUID) (int64, error) {
	return r.requeueRows, nil
}
func (r *fakeAdminRepo) CountPaperSections(ctx context.Context, paperID uuid.UUID) (int64, error) {
	return r.sectionCount, nil
}
func (r *fakeAdminRepo) GetJob(ctx context.Context, jobID uuid.UUID) (JobStatus, error) {
	return r.job, nil
}
func (r *fakeAdminRepo) SetJobTaskID(ctx context.Context, jobID uuid.UUID, taskID string) error {
	r.taskIDSet = taskID
	return nil
}

type fakeAdminStore struct {
	deleted []string
	err     error
}

func (s *fakeAdminStore) Delete(ctx context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return s.err
}

type fakeAdminEnqueuer struct {
	readCalled    bool
	processCalled bool
}

func (e *fakeAdminEnqueuer) EnqueuePaperProcessing(ctx context.Context, paperID, jobID uuid.UUID) (string, error) {
	e.processCalled = true
	return "task-process", nil
}
func (e *fakeAdminEnqueuer) EnqueuePaperRead(ctx context.Context, paperID, jobID uuid.UUID) (string, error) {
	e.readCalled = true
	return "task-read", nil
}

func TestDeletePaperRemovesObjectsAndRow(t *testing.T) {
	repo := &fakeAdminRepo{
		assets:     []AssetRef{{Bucket: "b", Key: "k1"}, {Bucket: "b", Key: "k2"}},
		deleteRows: 1,
	}
	store := &fakeAdminStore{}
	err := NewAdminService(repo, store, &fakeAdminEnqueuer{}).DeletePaper(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("DeletePaper error: %v", err)
	}
	if len(store.deleted) != 2 || store.deleted[0] != "k1" || store.deleted[1] != "k2" {
		t.Fatalf("deleted keys = %v, want [k1 k2]", store.deleted)
	}
}

func TestDeletePaperNotFound(t *testing.T) {
	repo := &fakeAdminRepo{assets: nil, deleteRows: 0}
	err := NewAdminService(repo, &fakeAdminStore{}, &fakeAdminEnqueuer{}).DeletePaper(context.Background(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDeletePaperToleratesStoreError(t *testing.T) {
	repo := &fakeAdminRepo{assets: []AssetRef{{Key: "k1"}}, deleteRows: 1}
	store := &fakeAdminStore{err: errors.New("minio down")}
	// Row is already gone; a store-delete failure must NOT fail the request.
	if err := NewAdminService(repo, store, &fakeAdminEnqueuer{}).DeletePaper(context.Background(), uuid.New()); err != nil {
		t.Fatalf("store error must not fail delete, got %v", err)
	}
}

func TestReprocessRequeuesParseStage(t *testing.T) {
	repo := &fakeAdminRepo{job: JobStatus{Status: "completed"}, requeueRows: 1}
	enq := &fakeAdminEnqueuer{}
	if _, err := NewAdminService(repo, &fakeAdminStore{}, enq).Reprocess(context.Background(), uuid.New()); err != nil {
		t.Fatalf("Reprocess error: %v", err)
	}
	if !enq.processCalled || enq.readCalled {
		t.Fatal("reprocess must enqueue parse, not read")
	}
	if repo.taskIDSet != "task-process" {
		t.Fatalf("task id = %q", repo.taskIDSet)
	}
}

func TestReprocessNotFound(t *testing.T) {
	repo := &fakeAdminRepo{jobErr: ErrNotFound}
	_, err := NewAdminService(repo, &fakeAdminStore{}, &fakeAdminEnqueuer{}).Reprocess(context.Background(), uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestRegenerateCardRequeuesReadStage(t *testing.T) {
	repo := &fakeAdminRepo{job: JobStatus{Status: "completed"}, requeueRows: 1, sectionCount: 7}
	enq := &fakeAdminEnqueuer{}
	if _, err := NewAdminService(repo, &fakeAdminStore{}, enq).RegenerateCard(context.Background(), uuid.New()); err != nil {
		t.Fatalf("RegenerateCard error: %v", err)
	}
	if !enq.readCalled || enq.processCalled {
		t.Fatal("regenerate must enqueue read, not parse")
	}
}

func TestRegenerateCardRejectsUnparsed(t *testing.T) {
	repo := &fakeAdminRepo{job: JobStatus{Status: "queued"}, sectionCount: 0}
	enq := &fakeAdminEnqueuer{}
	_, err := NewAdminService(repo, &fakeAdminStore{}, enq).RegenerateCard(context.Background(), uuid.New())
	if !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("want ErrNotRetryable, got %v", err)
	}
	if enq.readCalled {
		t.Fatal("must not enqueue read when there are no sections")
	}
}
