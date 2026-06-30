package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"scholarflow_server/internal/papers"
)

// PaperAdmin is the admin service surface used by the HTTP layer.
type PaperAdmin interface {
	DeletePaper(ctx context.Context, paperID uuid.UUID) error
	Reprocess(ctx context.Context, paperID uuid.UUID) (papers.JobStatus, error)
	RegenerateCard(ctx context.Context, paperID uuid.UUID) (papers.JobStatus, error)
}

type AdminHandler struct {
	admin PaperAdmin
}

func NewAdminHandler(admin PaperAdmin) *AdminHandler {
	return &AdminHandler{admin: admin}
}

func (h *AdminHandler) DeletePaper(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid paper id", http.StatusBadRequest)
		return
	}
	if err := h.admin.DeletePaper(r.Context(), id); err != nil {
		writeAdminError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) Reprocess(w http.ResponseWriter, r *http.Request) {
	h.requeue(w, r, h.admin.Reprocess)
}

func (h *AdminHandler) RegenerateCard(w http.ResponseWriter, r *http.Request) {
	h.requeue(w, r, h.admin.RegenerateCard)
}

func (h *AdminHandler) requeue(w http.ResponseWriter, r *http.Request, fn func(context.Context, uuid.UUID) (papers.JobStatus, error)) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid paper id", http.StatusBadRequest)
		return
	}
	job, err := fn(r.Context(), id)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(job)
}

func writeAdminError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, papers.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, papers.ErrNotRetryable):
		http.Error(w, "paper is not in a state that can be regenerated", http.StatusConflict)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
