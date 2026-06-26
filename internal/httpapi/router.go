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
	WriteAPIToken      string
}

func NewRouter(deps Dependencies) http.Handler {
	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Public read endpoints.
	if deps.ReadHandler != nil {
		r.Get("/v1/jobs/{id}", deps.ReadHandler.GetJob)
		r.Get("/v1/papers", deps.ReadHandler.ListPapers)
		r.Get("/v1/papers/{id}", deps.ReadHandler.GetPaper)
	}
	if deps.FigureImageHandler != nil {
		r.Get("/v1/papers/{id}/figures/{figureId}/image", deps.FigureImageHandler.GetFigureImage)
	}

	// Protected write endpoints. RequireToken is a pass-through when the
	// token is blank, so this group stays open until a token is configured.
	// Routes are always registered so RequireToken fires before route
	// matching for unauthenticated requests; a nil handler falls back to
	// http.NotFound (unreachable when the token is enforced).
	r.Group(func(pr chi.Router) {
		pr.Use(RequireToken(deps.WriteAPIToken))

		var uploadPaper http.HandlerFunc = http.NotFound
		if deps.UploadHandler != nil {
			uploadPaper = deps.UploadHandler.UploadPaper
		}
		pr.Post("/v1/uploads/papers", uploadPaper)

		var retry http.HandlerFunc = http.NotFound
		if deps.RetryHandler != nil {
			retry = deps.RetryHandler.Retry
		}
		pr.Post("/v1/jobs/{id}/retry", retry)

		var harvest http.HandlerFunc = http.NotFound
		if deps.HarvestHandler != nil {
			harvest = deps.HarvestHandler.TriggerArxiv
		}
		pr.Post("/v1/harvest/arxiv", harvest)
	})

	return r
}
