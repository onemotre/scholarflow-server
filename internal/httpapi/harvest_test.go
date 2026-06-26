package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeHarvestEnqueuer struct {
	gotCategories []string
	taskID        string
	err           error
}

func (e *fakeHarvestEnqueuer) EnqueueArxivHarvest(ctx context.Context, categories []string) (string, error) {
	e.gotCategories = categories
	return e.taskID, e.err
}

func TestTriggerArxivWithCategories(t *testing.T) {
	enq := &fakeHarvestEnqueuer{taskID: "task-123"}
	h := NewHarvestHandler(enq)
	req := httptest.NewRequest(http.MethodPost, "/v1/harvest/arxiv", strings.NewReader(`{"categories":["cs.CL","cs.AI"]}`))
	rec := httptest.NewRecorder()

	h.TriggerArxiv(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	var resp harvestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.TaskID != "task-123" {
		t.Fatalf("task_id = %q, want task-123", resp.TaskID)
	}
	if len(enq.gotCategories) != 2 || enq.gotCategories[0] != "cs.CL" || enq.gotCategories[1] != "cs.AI" {
		t.Fatalf("categories = %#v, want [cs.CL cs.AI]", enq.gotCategories)
	}
}

func TestTriggerArxivEmptyBodyUsesConfigured(t *testing.T) {
	enq := &fakeHarvestEnqueuer{taskID: "task-1"}
	h := NewHarvestHandler(enq)
	req := httptest.NewRequest(http.MethodPost, "/v1/harvest/arxiv", nil)
	rec := httptest.NewRecorder()

	h.TriggerArxiv(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	if len(enq.gotCategories) != 0 {
		t.Fatalf("categories = %#v, want empty (use configured)", enq.gotCategories)
	}
}

func TestTriggerArxivRejectsMalformedBody(t *testing.T) {
	h := NewHarvestHandler(&fakeHarvestEnqueuer{})
	req := httptest.NewRequest(http.MethodPost, "/v1/harvest/arxiv", strings.NewReader("{bad json"))
	rec := httptest.NewRecorder()

	h.TriggerArxiv(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
