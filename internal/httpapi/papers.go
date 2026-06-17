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

type PaperReader interface {
	GetJob(ctx context.Context, jobID uuid.UUID) (papers.JobStatus, error)
	GetPaperDetail(ctx context.Context, paperID uuid.UUID) (papers.PaperDetail, error)
}

type ReadHandler struct {
	reader PaperReader
}

func NewReadHandler(reader PaperReader) *ReadHandler {
	return &ReadHandler{reader: reader}
}

func (h *ReadHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid job id", http.StatusBadRequest)
		return
	}
	job, err := h.reader.GetJob(r.Context(), id)
	if err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, job)
}

func (h *ReadHandler) GetPaper(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid paper id", http.StatusBadRequest)
		return
	}
	paper, err := h.reader.GetPaperDetail(r.Context(), id)
	if err != nil {
		writeReadError(w, err)
		return
	}
	writeJSON(w, paper)
}

func writeReadError(w http.ResponseWriter, err error) {
	if errors.Is(err, papers.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.Error(w, "internal error", http.StatusInternalServerError)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
