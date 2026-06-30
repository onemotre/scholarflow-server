package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"scholarflow_server/internal/settings"
)

// SettingsService is the provider surface used by the HTTP layer.
type SettingsService interface {
	Effective(ctx context.Context) []settings.EffectiveSetting
	Set(ctx context.Context, key, value string) error
	Reset(ctx context.Context, key string) error
}

type SettingsHandler struct {
	svc SettingsService
}

func NewSettingsHandler(svc SettingsService) *SettingsHandler {
	return &SettingsHandler{svc: svc}
}

type settingsListResponse struct {
	Settings []settings.EffectiveSetting `json:"settings"`
}

// List returns all effective settings, masking secret values (value omitted;
// only is_set/source exposed).
func (h *SettingsHandler) List(w http.ResponseWriter, r *http.Request) {
	eff := h.svc.Effective(r.Context())
	masked := make([]settings.EffectiveSetting, len(eff))
	for i, s := range eff {
		if s.Secret {
			s.Value = ""
		}
		masked[i] = s
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(settingsListResponse{Settings: masked})
}

type settingsPutRequest struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (h *SettingsHandler) Put(w http.ResponseWriter, r *http.Request) {
	var req settingsPutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.svc.Set(r.Context(), req.Key, req.Value); err != nil {
		writeSettingsError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *SettingsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}
	if err := h.svc.Reset(r.Context(), key); err != nil {
		writeSettingsError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeSettingsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, settings.ErrUnknownKey):
		http.Error(w, "unknown setting key", http.StatusNotFound)
	case errors.Is(err, settings.ErrNotEditable):
		http.Error(w, "setting is not editable", http.StatusConflict)
	case errors.Is(err, settings.ErrInvalidValue):
		http.Error(w, "invalid setting value", http.StatusBadRequest)
	default:
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
