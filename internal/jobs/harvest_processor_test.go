package jobs

import (
	"context"
	"testing"

	"github.com/hibiken/asynq"
)

type fakeHarvestRunner struct {
	calls      int
	categories []string
}

func (r *fakeHarvestRunner) Harvest(ctx context.Context, categories []string) error {
	r.calls++
	r.categories = categories
	return nil
}

func TestHarvestProcessorPassesPayloadCategories(t *testing.T) {
	runner := &fakeHarvestRunner{}
	processor := NewHarvestProcessor(runner)
	task, err := NewHarvestArxivTask([]string{"cs.AI", "cs.LG"})
	if err != nil {
		t.Fatalf("NewHarvestArxivTask error: %v", err)
	}
	if err := processor.HandleHarvest(context.Background(), task); err != nil {
		t.Fatalf("HandleHarvest error: %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("Harvest calls = %d, want 1", runner.calls)
	}
	if len(runner.categories) != 2 || runner.categories[0] != "cs.AI" || runner.categories[1] != "cs.LG" {
		t.Fatalf("categories = %#v, want [cs.AI cs.LG]", runner.categories)
	}
}

func TestHarvestProcessorNilPayloadCategories(t *testing.T) {
	runner := &fakeHarvestRunner{}
	processor := NewHarvestProcessor(runner)
	task, err := NewHarvestArxivTask(nil) // the scheduled-cron path
	if err != nil {
		t.Fatalf("NewHarvestArxivTask error: %v", err)
	}
	if err := processor.HandleHarvest(context.Background(), task); err != nil {
		t.Fatalf("HandleHarvest error: %v", err)
	}
	if len(runner.categories) != 0 {
		t.Fatalf("categories = %#v, want empty (fall back to configured)", runner.categories)
	}
}

func TestHarvestProcessorRejectsMalformedPayload(t *testing.T) {
	processor := NewHarvestProcessor(&fakeHarvestRunner{})
	task := asynq.NewTask(TypeHarvestArxiv, []byte("{bad json"))
	if err := processor.HandleHarvest(context.Background(), task); err == nil {
		t.Fatal("HandleHarvest returned nil, want error for malformed payload")
	}
}
