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
	EvidenceType string  `json:"evidence_type"`
	SectionID    string  `json:"section_id,omitempty"`
	AssetID      string  `json:"asset_id,omitempty"`
	Page         *int    `json:"page,omitempty"`
	Locator      string  `json:"locator,omitempty"`
	Snippet      string  `json:"snippet,omitempty"`
	Confidence   float64 `json:"confidence"`
}

type PaperCard struct {
	Background     string     `json:"background"`
	Problem        string     `json:"problem"`
	Method         string     `json:"method"`
	Implementation string     `json:"implementation"`
	Benchmarks     []string   `json:"benchmarks"`
	Baselines      []string   `json:"baselines"`
	Results        []string   `json:"results"`
	CodeLinks      []string   `json:"code_links"`
	DataLinks      []string   `json:"data_links"`
	KeyFigures     []string   `json:"key_figures"`
	Evidence       []Evidence `json:"evidence"`
}

type Context struct {
	Title    string
	Abstract string
	Sections []Section
	Figures  []Figure
}

type Section struct {
	Label   string
	Heading string
	Text    string
}

type Figure struct {
	Label   string
	Kind    string
	Caption string
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
