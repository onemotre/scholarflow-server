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

type OpenAIReader struct {
	baseURL       string
	apiKey        string
	model         string
	maxInputChars int
	client        *http.Client
}

func NewOpenAIReader(baseURL, apiKey, model string, maxInputChars int, timeout time.Duration) *OpenAIReader {
	return &OpenAIReader{
		baseURL:       strings.TrimRight(baseURL, "/"),
		apiKey:        apiKey,
		model:         model,
		maxInputChars: maxInputChars,
		client:        &http.Client{Timeout: timeout},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	Temperature    float64        `json:"temperature"`
	ResponseFormat map[string]any `json:"response_format,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

func (r *OpenAIReader) ReadPaper(ctx context.Context, input Context) (PaperCard, error) {
	reqBody := chatRequest{
		Model: r.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: buildUserPrompt(input, r.maxInputChars)},
		},
		Temperature:    0,
		ResponseFormat: map[string]any{"type": "json_object"},
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		content, err := r.call(ctx, reqBody)
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

func (r *OpenAIReader) call(ctx context.Context, body chatRequest) (string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL+"/chat/completions", bytes.NewReader(payload))
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
	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("decode openai response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai response had no choices")
	}
	return parsed.Choices[0].Message.Content, nil
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
