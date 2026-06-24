package reader

import (
	"context"
	"errors"
	"fmt"
)

// ErrDisallowedKey is returned when a card contains a field that is not permitted
// by the schema (e.g. "limitations").
var ErrDisallowedKey = errors.New("limitations field is not allowed")

type Evidence struct {
	ClaimKey     string  `json:"claim_key"`
	ClaimIndex   *int    `json:"claim_index,omitempty"`
	EvidenceType string  `json:"evidence_type"`
	SectionID    string  `json:"section_id,omitempty"`
	AssetID      string  `json:"asset_id,omitempty"`
	Page         *int    `json:"page,omitempty"`
	Locator      string  `json:"locator,omitempty"`
	Snippet      string  `json:"snippet,omitempty"`
	Confidence   float64 `json:"confidence"`
}

// FigureRef places a figure (by its label) at a claim anchor. Page is resolved
// server-side from the GROBID figure record, not supplied by the model.
type FigureRef struct {
	Label      string `json:"label"`
	ClaimKey   string `json:"claim_key"`
	ClaimIndex *int   `json:"claim_index,omitempty"`
	Page       *int   `json:"page,omitempty"`
}

type PaperCard struct {
	Background     string      `json:"background"`
	Problem        string      `json:"problem"`
	Method         string      `json:"method"`
	Implementation string      `json:"implementation"`
	Benchmarks     []string    `json:"benchmarks"`
	Baselines      []string    `json:"baselines"`
	Results        []string    `json:"results"`
	CodeLinks      []string    `json:"code_links"`
	DataLinks      []string    `json:"data_links"`
	Figures        []FigureRef `json:"figures"`
	Evidence       []Evidence  `json:"evidence"`
}

type Context struct {
	Title    string
	Abstract string
	Sections []Section
	Figures  []Figure
}

type Section struct {
	Label     string
	Heading   string
	Text      string
	PageStart *int
	PageEnd   *int
}

type Figure struct {
	Label   string
	Kind    string
	Caption string
	Page    *int
}

type Reader interface {
	ReadPaper(ctx context.Context, input Context) (PaperCard, error)
}

func (c PaperCard) Validate() error {
	if c.Background == "" && c.Problem == "" && c.Method == "" && c.Implementation == "" {
		return fmt.Errorf("paper card has no core content")
	}
	return nil
}

func ValidateRawKeys(raw map[string]any) error {
	if _, ok := raw["limitations"]; ok {
		return ErrDisallowedKey
	}
	return nil
}

func cardJSONSchema() map[string]any {
	strArray := func() map[string]any {
		return map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
	}
	evidence := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"claim_key", "claim_index", "evidence_type", "section_id", "asset_id", "page", "locator", "snippet", "confidence"},
		"properties": map[string]any{
			"claim_key":     map[string]any{"type": "string"},
			"claim_index":   map[string]any{"type": []string{"integer", "null"}},
			"evidence_type": map[string]any{"type": "string"},
			"section_id":    map[string]any{"type": "string"},
			"asset_id":      map[string]any{"type": "string"},
			"page":          map[string]any{"type": []string{"integer", "null"}},
			"locator":       map[string]any{"type": "string"},
			"snippet":       map[string]any{"type": "string"},
			"confidence":    map[string]any{"type": "number"},
		},
	}
	figure := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"label", "claim_key", "claim_index"},
		"properties": map[string]any{
			"label":       map[string]any{"type": "string"},
			"claim_key":   map[string]any{"type": "string"},
			"claim_index": map[string]any{"type": []string{"integer", "null"}},
		},
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"background", "problem", "method", "implementation", "benchmarks", "baselines", "results", "code_links", "data_links", "figures", "evidence"},
		"properties": map[string]any{
			"background":     map[string]any{"type": "string"},
			"problem":        map[string]any{"type": "string"},
			"method":         map[string]any{"type": "string"},
			"implementation": map[string]any{"type": "string"},
			"benchmarks":     strArray(),
			"baselines":      strArray(),
			"results":        strArray(),
			"code_links":     strArray(),
			"data_links":     strArray(),
			"figures":        map[string]any{"type": "array", "items": figure},
			"evidence":       map[string]any{"type": "array", "items": evidence},
		},
	}
}
