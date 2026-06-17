package reader

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newCardServer(t *testing.T, bodies ...string) *httptest.Server {
	t.Helper()
	calls := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing bearer auth: %q", r.Header.Get("Authorization"))
		}
		body := bodies[calls]
		if calls < len(bodies)-1 {
			calls++
		}
		resp := map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": body}}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func newReader(url string) *OpenAIReader {
	return NewOpenAIReader(url, "test-key", "gpt-4o-mini", 48000, 5*time.Second)
}

func sampleContext() Context {
	return Context{
		Title:    "A Paper",
		Abstract: "We do things.",
		Sections: []Section{{Label: "1", Heading: "Intro", Text: "Body text"}},
	}
}

func TestOpenAIReaderParsesCard(t *testing.T) {
	card := `{"background":"bg","problem":"p","method":"m","implementation":"impl","evidence":[{"claim_key":"method","evidence_type":"section","section_id":"1","confidence":0.8}]}`
	srv := newCardServer(t, card)
	defer srv.Close()

	got, err := newReader(srv.URL).ReadPaper(context.Background(), sampleContext())
	if err != nil {
		t.Fatalf("ReadPaper error: %v", err)
	}
	if got.Method != "m" {
		t.Fatalf("method = %q", got.Method)
	}
	if len(got.Evidence) != 1 || got.Evidence[0].SectionID != "1" {
		t.Fatalf("evidence = %#v", got.Evidence)
	}
}

func TestOpenAIReaderRejectsLimitations(t *testing.T) {
	card := `{"background":"bg","problem":"p","method":"m","implementation":"impl","limitations":"nope"}`
	srv := newCardServer(t, card)
	defer srv.Close()

	_, err := newReader(srv.URL).ReadPaper(context.Background(), sampleContext())
	if err == nil || !strings.Contains(err.Error(), "limitations") {
		t.Fatalf("want limitations error, got %v", err)
	}
}

func TestOpenAIReaderRetriesBadJSON(t *testing.T) {
	good := `{"background":"bg","problem":"p","method":"m","implementation":"impl"}`
	srv := newCardServer(t, "not json", good)
	defer srv.Close()

	got, err := newReader(srv.URL).ReadPaper(context.Background(), sampleContext())
	if err != nil {
		t.Fatalf("ReadPaper error after retry: %v", err)
	}
	if got.Background != "bg" {
		t.Fatalf("background = %q", got.Background)
	}
}
