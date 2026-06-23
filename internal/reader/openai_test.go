package reader

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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
	return NewOpenAIReader(OpenAIConfig{
		BaseURL:        url,
		APIKey:         "test-key",
		Model:          "gpt-4o-mini",
		APIStyle:       "chat",
		ResponseFormat: "json_object",
		SystemPrompt:   "test-system-prompt",
		MaxInputChars:  48000,
		Timeout:        5 * time.Second,
	})
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
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		resp := map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": card}}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	_, err := newReader(srv.URL).ReadPaper(context.Background(), sampleContext())
	if !errors.Is(err, ErrDisallowedKey) {
		t.Fatalf("want ErrDisallowedKey, got %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("server calls = %d, want 1 (limitations must not be retried)", got)
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

func TestResponsesStyleParsesCard(t *testing.T) {
	card := `{"background":"bg","problem":"p","method":"m","implementation":"impl"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Errorf("path = %q, want /responses", r.URL.Path)
		}
		resp := map[string]any{"output": []map[string]any{
			{"content": []map[string]any{{"type": "output_text", "text": card}}},
		}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	rd := NewOpenAIReader(OpenAIConfig{
		BaseURL: srv.URL, APIKey: "test-key", Model: "m",
		APIStyle: "responses", ResponseFormat: "json_object",
		SystemPrompt: "sp", MaxInputChars: 48000, Timeout: 5 * time.Second,
	})
	got, err := rd.ReadPaper(context.Background(), sampleContext())
	if err != nil {
		t.Fatalf("ReadPaper error: %v", err)
	}
	if got.Background != "bg" {
		t.Fatalf("background = %q", got.Background)
	}
}

func TestResponsesStyleHonorsOutputText(t *testing.T) {
	card := `{"background":"bg","problem":"p","method":"m","implementation":"impl"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"output_text": card, "output": []any{}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	rd := NewOpenAIReader(OpenAIConfig{
		BaseURL: srv.URL, APIKey: "test-key", Model: "m",
		APIStyle: "responses", ResponseFormat: "json_object",
		SystemPrompt: "sp", MaxInputChars: 48000, Timeout: 5 * time.Second,
	})
	if _, err := rd.ReadPaper(context.Background(), sampleContext()); err != nil {
		t.Fatalf("ReadPaper error: %v", err)
	}
}

func TestChatJSONSchemaRequestShape(t *testing.T) {
	card := `{"background":"bg","problem":"p","method":"m","implementation":"impl"}`
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		resp := map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": card}}}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	rd := NewOpenAIReader(OpenAIConfig{
		BaseURL: srv.URL, APIKey: "test-key", Model: "m",
		APIStyle: "chat", ResponseFormat: "json_schema",
		SystemPrompt: "sp", MaxInputChars: 48000, Timeout: 5 * time.Second,
	})
	if _, err := rd.ReadPaper(context.Background(), sampleContext()); err != nil {
		t.Fatalf("ReadPaper error: %v", err)
	}
	rf, ok := gotBody["response_format"].(map[string]any)
	if !ok {
		t.Fatalf("response_format missing: %#v", gotBody)
	}
	if rf["type"] != "json_schema" {
		t.Fatalf("response_format.type = %v", rf["type"])
	}
	js, ok := rf["json_schema"].(map[string]any)
	if !ok || js["strict"] != true || js["name"] != "paper_card" {
		t.Fatalf("json_schema block = %#v", rf["json_schema"])
	}
}
