package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Dependencies struct {
	UploadHandler *UploadHandler
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
	return r
}
