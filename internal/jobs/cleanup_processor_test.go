package jobs

import (
	"context"
	"testing"
	"time"
)

type fakeCleanupRepo struct {
	cutoff  time.Time
	deleted int64
}

func (r *fakeCleanupRepo) DeleteFailedJobsOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	r.cutoff = cutoff
	return r.deleted, nil
}

func TestCleanupProcessorDeletesWithRetentionCutoff(t *testing.T) {
	repo := &fakeCleanupRepo{deleted: 3}
	proc := NewCleanupProcessor(repo, 7)
	before := time.Now().Add(-7 * 24 * time.Hour)

	task, err := NewCleanupJobsTask()
	if err != nil {
		t.Fatalf("NewCleanupJobsTask error: %v", err)
	}
	if err := proc.HandleCleanup(context.Background(), task); err != nil {
		t.Fatalf("HandleCleanup error: %v", err)
	}
	after := time.Now().Add(-7 * 24 * time.Hour)

	if repo.cutoff.Before(before.Add(-time.Minute)) || repo.cutoff.After(after.Add(time.Minute)) {
		t.Fatalf("cutoff %v not within ~7 days ago", repo.cutoff)
	}
}

func TestNewCleanupJobsTaskType(t *testing.T) {
	task, err := NewCleanupJobsTask()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if task.Type() != TypeCleanupJobs {
		t.Fatalf("type = %q, want %q", task.Type(), TypeCleanupJobs)
	}
}
