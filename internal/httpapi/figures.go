package httpapi

import (
	"context"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type FigureImageReader interface {
	GetFigureImageKey(ctx context.Context, paperID, figureID uuid.UUID) (string, error)
}

type ObjectStore interface {
	Get(ctx context.Context, key string) (io.ReadCloser, error)
}

type FigureImageHandler struct {
	reader FigureImageReader
	store  ObjectStore
}

func NewFigureImageHandler(reader FigureImageReader, store ObjectStore) *FigureImageHandler {
	return &FigureImageHandler{reader: reader, store: store}
}

func (h *FigureImageHandler) GetFigureImage(w http.ResponseWriter, r *http.Request) {
	paperID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid paper id", http.StatusBadRequest)
		return
	}
	figureID, err := uuid.Parse(chi.URLParam(r, "figureId"))
	if err != nil {
		http.Error(w, "invalid figure id", http.StatusBadRequest)
		return
	}
	key, err := h.reader.GetFigureImageKey(r.Context(), paperID, figureID)
	if err != nil {
		writeReadError(w, err) // maps papers.ErrNotFound -> 404
		return
	}
	obj, err := h.store.Get(r.Context(), key)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer obj.Close()
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, obj)
}
