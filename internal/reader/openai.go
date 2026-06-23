package reader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAIConfig struct {
	BaseURL        string
	APIKey         string
	Model          string
	APIStyle       string // "chat" | "responses"
	ResponseFormat string // "json_schema" | "json_object"
	SystemPrompt   string
	MaxInputChars  int
	Timeout        time.Duration
}

type OpenAIReader struct {
	baseURL        string
	apiKey         string
	model          string
	apiStyle       string
	responseFormat string
	systemPrompt   string
	maxInputChars  int
	client         *http.Client
}

func NewOpenAIReader(cfg OpenAIConfig) *OpenAIReader {
	return &OpenAIReader{
		baseURL:        strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:         cfg.APIKey,
		model:          cfg.Model,
		apiStyle:       normalizeAPIStyle(cfg.APIStyle),
		responseFormat: normalizeResponseFormat(cfg.ResponseFormat),
		systemPrompt:   cfg.SystemPrompt,
		maxInputChars:  cfg.MaxInputChars,
		client:         &http.Client{Timeout: cfg.Timeout},
	}
}

func normalizeAPIStyle(s string) string {
	if strings.EqualFold(strings.TrimSpace(s), "responses") {
		return "responses"
	}
	return "chat"
}

func normalizeResponseFormat(s string) string {
	if strings.EqualFold(strings.TrimSpace(s), "json_object") {
		return "json_object"
	}
	return "json_schema"
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (r *OpenAIReader) ReadPaper(ctx context.Context, input Context) (PaperCard, error) {
	user := buildUserPrompt(input, r.maxInputChars)
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		content, err := r.call(ctx, r.systemPrompt, user)
		if err != nil {
			return PaperCard{}, err
		}
		card, err := parseCard(content)
		if err == nil {
			return card, nil
		}
		if errors.Is(err, ErrDisallowedKey) {
			return PaperCard{}, err
		}
		lastErr = err
	}
	return PaperCard{}, fmt.Errorf("read paper: %w", lastErr)
}

func (r *OpenAIReader) call(ctx context.Context, system, user string) (string, error) {
	path, body := r.buildRequest(system, user)
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.apiKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("openai returned status %d: %s", resp.StatusCode, string(raw))
	}
	if r.apiStyle == "responses" {
		return extractResponsesContent(raw)
	}
	return extractChatContent(raw)
}

func (r *OpenAIReader) buildRequest(system, user string) (string, any) {
	messages := []chatMessage{{Role: "system", Content: system}, {Role: "user", Content: user}}
	if r.apiStyle == "responses" {
		return "/responses", map[string]any{
			"model":       r.model,
			"input":       messages,
			"temperature": 0,
			"text":        map[string]any{"format": r.formatSpec()},
		}
	}
	return "/chat/completions", map[string]any{
		"model":           r.model,
		"messages":        messages,
		"temperature":     0,
		"response_format": r.formatSpec(),
	}
}

// formatSpec builds the response-format block for the active style + format.
func (r *OpenAIReader) formatSpec() map[string]any {
	if r.responseFormat != "json_schema" {
		return map[string]any{"type": "json_object"}
	}
	if r.apiStyle == "responses" {
		return map[string]any{
			"type":   "json_schema",
			"name":   "paper_card",
			"strict": true,
			"schema": cardJSONSchema(),
		}
	}
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "paper_card",
			"strict": true,
			"schema": cardJSONSchema(),
		},
	}
}

func extractChatContent(raw []byte) (string, error) {
	var parsed struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai response had no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

func extractResponsesContent(raw []byte) (string, error) {
	var parsed struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}
	if strings.TrimSpace(parsed.OutputText) != "" {
		return parsed.OutputText, nil
	}
	var b strings.Builder
	for _, item := range parsed.Output {
		for _, c := range item.Content {
			if c.Text != "" {
				b.WriteString(c.Text)
			}
		}
	}
	if b.Len() == 0 {
		return "", fmt.Errorf("openai responses output had no text")
	}
	return b.String(), nil
}

func parseCard(content string) (PaperCard, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return PaperCard{}, fmt.Errorf("parse card json: %w", err)
	}
	if err := ValidateRawKeys(raw); err != nil {
		return PaperCard{}, err
	}
	var card PaperCard
	if err := json.Unmarshal([]byte(content), &card); err != nil {
		return PaperCard{}, fmt.Errorf("unmarshal card: %w", err)
	}
	if err := card.Validate(); err != nil {
		return PaperCard{}, err
	}
	return card, nil
}
