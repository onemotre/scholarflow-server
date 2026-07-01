package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPanelPageServesHTML(t *testing.T) {
	srv := httptest.NewServer(NewRouter(Dependencies{PanelHandler: NewPanelHandler()}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/panel")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type = %q, want text/html", ct)
	}
}

func TestPanelStaticServesCSS(t *testing.T) {
	srv := httptest.NewServer(NewRouter(Dependencies{PanelHandler: NewPanelHandler()}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/panel/static/panel.css")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/css") {
		t.Fatalf("content-type = %q, want text/css", ct)
	}
}

func TestPanelStaticServesSettingsJS(t *testing.T) {
	srv := httptest.NewServer(NewRouter(Dependencies{PanelHandler: NewPanelHandler()}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/panel/static/settings.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
