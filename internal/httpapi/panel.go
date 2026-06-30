package httpapi

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var panelAssets embed.FS

// PanelHandler serves the control panel HTML page and its static assets.
type PanelHandler struct{}

func NewPanelHandler() *PanelHandler { return &PanelHandler{} }

// Page serves the panel HTML shell.
func (h *PanelHandler) Page(w http.ResponseWriter, r *http.Request) {
	data, err := panelAssets.ReadFile("static/panel.html")
	if err != nil {
		http.Error(w, "panel unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// Static returns a handler that serves files under static/ at /panel/static/.
func (h *PanelHandler) Static() http.Handler {
	sub, err := fs.Sub(panelAssets, "static")
	if err != nil {
		panic(err) // embedded path is constant; failure is a build-time bug
	}
	return http.StripPrefix("/panel/static/", http.FileServer(http.FS(sub)))
}
