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

type MethodologyItem struct {
	Problem string `json:"problem"`
	Method  string `json:"method"`
}

type Comparison struct {
	Work      string `json:"work"`
	Value     string `json:"value"`
	Reference string `json:"reference"`
}

type ResultItem struct {
	Metric      string       `json:"metric"`
	Finding     string       `json:"finding"`
	Comparisons []Comparison `json:"comparisons"`
	SelfOnly    bool         `json:"self_only"`
}

type Module struct {
	Name      string `json:"name"`
	Function  string `json:"function"`
	Design    string `json:"design"`
	Principle string `json:"principle"`
}

type Implementation struct {
	Overview string   `json:"overview"`
	Modules  []Module `json:"modules"`
}

type PaperCard struct {
	Introduction   string            `json:"introduction"`
	RelatedWork    string            `json:"related_work"`
	Methodology    []MethodologyItem `json:"methodology"`
	Results        []ResultItem      `json:"results"`
	Implementation Implementation    `json:"implementation"`
	CodeLinks      []string          `json:"code_links"`
	DataLinks      []string          `json:"data_links"`
	Figures        []FigureRef       `json:"figures"`
	Evidence       []Evidence        `json:"evidence"`
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
	if c.Introduction == "" && len(c.Methodology) == 0 {
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
	str := func() map[string]any { return map[string]any{"type": "string"} }
	strArray := func() map[string]any {
		return map[string]any{"type": "array", "items": str()}
	}
	obj := func(required []string, props map[string]any) map[string]any {
		return map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             required,
			"properties":           props,
		}
	}
	arrayOf := func(item map[string]any) map[string]any {
		return map[string]any{"type": "array", "items": item}
	}

	methodologyItem := obj([]string{"problem", "method"}, map[string]any{
		"problem": str(),
		"method":  str(),
	})
	comparison := obj([]string{"work", "value", "reference"}, map[string]any{
		"work":      str(),
		"value":     str(),
		"reference": str(),
	})
	resultItem := obj([]string{"metric", "finding", "comparisons", "self_only"}, map[string]any{
		"metric":      str(),
		"finding":     str(),
		"comparisons": arrayOf(comparison),
		"self_only":   map[string]any{"type": "boolean"},
	})
	module := obj([]string{"name", "function", "design", "principle"}, map[string]any{
		"name":      str(),
		"function":  str(),
		"design":    str(),
		"principle": str(),
	})
	implementation := obj([]string{"overview", "modules"}, map[string]any{
		"overview": str(),
		"modules":  arrayOf(module),
	})
	evidence := obj(
		[]string{"claim_key", "claim_index", "evidence_type", "section_id", "asset_id", "page", "locator", "snippet", "confidence"},
		map[string]any{
			"claim_key":     str(),
			"claim_index":   map[string]any{"type": []string{"integer", "null"}},
			"evidence_type": str(),
			"section_id":    str(),
			"asset_id":      str(),
			"page":          map[string]any{"type": []string{"integer", "null"}},
			"locator":       str(),
			"snippet":       str(),
			"confidence":    map[string]any{"type": "number"},
		},
	)
	figure := obj([]string{"label", "claim_key", "claim_index"}, map[string]any{
		"label":       str(),
		"claim_key":   str(),
		"claim_index": map[string]any{"type": []string{"integer", "null"}},
	})
	return obj(
		[]string{"introduction", "related_work", "methodology", "results", "implementation", "code_links", "data_links", "figures", "evidence"},
		map[string]any{
			"introduction":   str(),
			"related_work":   str(),
			"methodology":    arrayOf(methodologyItem),
			"results":        arrayOf(resultItem),
			"implementation": implementation,
			"code_links":     strArray(),
			"data_links":     strArray(),
			"figures":        arrayOf(figure),
			"evidence":       arrayOf(evidence),
		},
	)
}
