package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
)

// HarvestEnqueuer enqueues an on-demand arxiv harvest task.
type HarvestEnqueuer interface {
	EnqueueArxivHarvest(ctx context.Context, categories []string) (string, error)
}

type HarvestHandler struct {
	enqueuer HarvestEnqueuer
}

func NewHarvestHandler(enqueuer HarvestEnqueuer) *HarvestHandler {
	return &HarvestHandler{enqueuer: enqueuer}
}

type harvestRequest struct {
	Categories []string `json:"categories"`
}

type harvestResponse struct {
	TaskID string `json:"task_id"`
}

// TriggerArxiv enqueues an arxiv harvest. The body is optional; when it carries
// a non-empty "categories" array, those override the worker's configured
// categories for this run. An empty/absent body harvests the configured ones.
func (h *HarvestHandler) TriggerArxiv(w http.ResponseWriter, r *http.Request) {
	var req harvestRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
	}
	taskID, err := h.enqueuer.EnqueueArxivHarvest(r.Context(), req.Categories)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(harvestResponse{TaskID: taskID})
}
