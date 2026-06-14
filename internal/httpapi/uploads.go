package httpapi

import (
	"encoding/json"
	"net/http"

	"scholarflow_server/internal/papers"
)

type UploadHandler struct {
	service        *papers.Service
	maxUploadBytes int64
}

func NewUploadHandler(service *papers.Service, maxUploadBytes int64) *UploadHandler {
	return &UploadHandler{service: service, maxUploadBytes: maxUploadBytes}
}

func (h *UploadHandler) UploadPaper(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)
	if err := r.ParseMultipartForm(h.maxUploadBytes); err != nil {
		http.Error(w, "invalid multipart upload", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing file field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	result, err := h.service.UploadPDF(r.Context(), header.Filename, file, header.Size, contentType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(result)
}
