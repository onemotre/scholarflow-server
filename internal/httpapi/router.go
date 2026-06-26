package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Dependencies struct {
	UploadHandler      *UploadHandler
	ReadHandler        *ReadHandler
	RetryHandler       *RetryHandler
	FigureImageHandler *FigureImageHandler
	HarvestHandler     *HarvestHandler
}

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if deps.UploadHandler != nil {
		r.Post("/v1/uploads/papers", deps.UploadHandler.UploadPaper)
	}
	if deps.ReadHandler != nil {
		r.Get("/v1/jobs/{id}", deps.ReadHandler.GetJob)
		r.Get("/v1/papers", deps.ReadHandler.ListPapers)
		r.Get("/v1/papers/{id}", deps.ReadHandler.GetPaper)
	}
	if deps.RetryHandler != nil {
		r.Post("/v1/jobs/{id}/retry", deps.RetryHandler.Retry)
	}
	if deps.FigureImageHandler != nil {
		r.Get("/v1/papers/{id}/figures/{figureId}/image", deps.FigureImageHandler.GetFigureImage)
	}
	if deps.HarvestHandler != nil {
		r.Post("/v1/harvest/arxiv", deps.HarvestHandler.TriggerArxiv)
	}
	return r
}
