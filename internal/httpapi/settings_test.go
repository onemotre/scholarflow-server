package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"scholarflow_server/internal/settings"
)

type fakeSettings struct {
	eff       []settings.EffectiveSetting
	setErr    error
	resetErr  error
	gotSetKey string
	gotSetVal string
	gotDelKey string
}

func (f *fakeSettings) Effective(ctx context.Context) []settings.EffectiveSetting { return f.eff }
func (f *fakeSettings) Set(ctx context.Context, key, value string) error {
	f.gotSetKey, f.gotSetVal = key, value
	return f.setErr
}
func (f *fakeSettings) Reset(ctx context.Context, key string) error {
	f.gotDelKey = key
	return f.resetErr
}

func newSettingsServer(svc SettingsService) *httptest.Server {
	return httptest.NewServer(NewRouter(Dependencies{SettingsHandler: NewSettingsHandler(svc)}))
}

func TestSettingsListMasksSecrets(t *testing.T) {
	svc := &fakeSettings{eff: []settings.EffectiveSetting{
		{Key: "OPENAI_MODEL", Secret: false, Value: "gpt-x", IsSet: true, Source: "db"},
		{Key: "OPENAI_API_KEY", Secret: true, Value: "sk-supersecret", IsSet: true, Source: "db"},
	}}
	srv := newSettingsServer(svc)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "sk-supersecret") {
		t.Fatal("secret value leaked in GET /v1/settings")
	}
	if !strings.Contains(string(body), "gpt-x") {
		t.Fatal("non-secret value should be present")
	}
	// secret must still report is_set
	var payload struct {
		Settings []settings.EffectiveSetting `json:"settings"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	for _, s := range payload.Settings {
		if s.Key == "OPENAI_API_KEY" {
			if s.Value != "" || !s.IsSet {
				t.Fatalf("secret leaked or is_set wrong: %+v", s)
			}
		}
	}
}

func TestSettingsPutValid(t *testing.T) {
	svc := &fakeSettings{}
	srv := newSettingsServer(svc)
	defer srv.Close()
	body := strings.NewReader(`{"key":"OPENAI_MODEL","value":"gpt-x"}`)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/settings", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if svc.gotSetKey != "OPENAI_MODEL" || svc.gotSetVal != "gpt-x" {
		t.Fatalf("set got %q=%q", svc.gotSetKey, svc.gotSetVal)
	}
}

func TestSettingsPutInvalidValue(t *testing.T) {
	svc := &fakeSettings{setErr: settings.ErrInvalidValue}
	srv := newSettingsServer(svc)
	defer srv.Close()
	body := strings.NewReader(`{"key":"READ_MAX_RETRY","value":"nope"}`)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/settings", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSettingsPutBootstrapRejected(t *testing.T) {
	svc := &fakeSettings{setErr: settings.ErrNotEditable}
	srv := newSettingsServer(svc)
	defer srv.Close()
	body := strings.NewReader(`{"key":"DATABASE_URL","value":"x"}`)
	req, _ := http.NewRequest(http.MethodPut, srv.URL+"/v1/settings", body)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestSettingsDeleteResets(t *testing.T) {
	svc := &fakeSettings{}
	srv := newSettingsServer(svc)
	defer srv.Close()
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/v1/settings/OPENAI_MODEL", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if svc.gotDelKey != "OPENAI_MODEL" {
		t.Fatalf("reset key = %q", svc.gotDelKey)
	}
}
