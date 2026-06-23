package papers

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

type fakeRetryRepo struct {
	job          JobStatus
	getErr       error
	sectionCount int64
	resetRows    int64
	taskIDSet    string
}

func (r *fakeRetryRepo) GetJob(ctx context.Context, jobID uuid.UUID) (JobStatus, error) {
	if r.getErr != nil {
		return JobStatus{}, r.getErr
	}
	return r.job, nil
}
func (r *fakeRetryRepo) CountPaperSections(ctx context.Context, paperID uuid.UUID) (int64, error) {
	return r.sectionCount, nil
}
func (r *fakeRetryRepo) ResetFailedJob(ctx context.Context, jobID uuid.UUID) (int64, error) {
	return r.resetRows, nil
}
func (r *fakeRetryRepo) SetJobTaskID(ctx context.Context, jobID uuid.UUID, taskID string) error {
	r.taskIDSet = taskID
	return nil
}

type fakeRetryEnqueuer struct {
	readCalled    bool
	processCalled bool
}

func (e *fakeRetryEnqueuer) EnqueuePaperRead(ctx context.Context, paperID, jobID uuid.UUID) (string, error) {
	e.readCalled = true
	return "task-read", nil
}
func (e *fakeRetryEnqueuer) EnqueuePaperProcessing(ctx context.Context, paperID, jobID uuid.UUID) (string, error) {
	e.processCalled = true
	return "task-process", nil
}

func TestRetryJobRejectsNonFailed(t *testing.T) {
	repo := &fakeRetryRepo{job: JobStatus{Status: "completed"}}
	enq := &fakeRetryEnqueuer{}
	_, err := NewRetryService(repo, enq).RetryJob(context.Background(), uuid.New())
	if !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("want ErrNotRetryable, got %v", err)
	}
	if enq.readCalled || enq.processCalled {
		t.Fatal("must not enqueue a non-failed job")
	}
}

func TestRetryJobReadStageWhenParsed(t *testing.T) {
	repo := &fakeRetryRepo{job: JobStatus{Status: "failed"}, sectionCount: 5, resetRows: 1}
	enq := &fakeRetryEnqueuer{}
	if _, err := NewRetryService(repo, enq).RetryJob(context.Background(), uuid.New()); err != nil {
		t.Fatalf("RetryJob error: %v", err)
	}
	if !enq.readCalled || enq.processCalled {
		t.Fatal("parsed paper must re-enqueue read, not parse")
	}
	if repo.taskIDSet != "task-read" {
		t.Fatalf("task id = %q", repo.taskIDSet)
	}
}

func TestRetryJobParseStageWhenNotParsed(t *testing.T) {
	repo := &fakeRetryRepo{job: JobStatus{Status: "failed"}, sectionCount: 0, resetRows: 1}
	enq := &fakeRetryEnqueuer{}
	if _, err := NewRetryService(repo, enq).RetryJob(context.Background(), uuid.New()); err != nil {
		t.Fatalf("RetryJob error: %v", err)
	}
	if !enq.processCalled || enq.readCalled {
		t.Fatal("unparsed paper must re-enqueue parse, not read")
	}
	if repo.taskIDSet != "task-process" {
		t.Fatalf("task id = %q, want task-process", repo.taskIDSet)
	}
}

func TestRetryJobRejectsLostRace(t *testing.T) {
	repo := &fakeRetryRepo{job: JobStatus{Status: "failed"}, sectionCount: 5, resetRows: 0}
	enq := &fakeRetryEnqueuer{}
	_, err := NewRetryService(repo, enq).RetryJob(context.Background(), uuid.New())
	if !errors.Is(err, ErrNotRetryable) {
		t.Fatalf("want ErrNotRetryable on 0 rows reset, got %v", err)
	}
	if enq.readCalled || enq.processCalled {
		t.Fatal("must not enqueue when reset affected 0 rows")
	}
}
