package reader

import (
	"fmt"
	"strings"
)

const systemPrompt = `You are an objective research-paper summarizer. Produce a strict JSON object describing the paper.
Rules:
- Output ONLY a JSON object, no prose, no code fences.
- Never include a "limitations" field.
- Be factual; do not speculate. Use empty strings or empty arrays when unknown.
- For each non-trivial claim, add an "evidence" entry whose "section_id" is the SECTION LABEL (the number shown in brackets) that supports it.
JSON shape:
{"background":"","problem":"","method":"","implementation":"","benchmarks":[],"baselines":[],"results":[],"code_links":[],"data_links":[],"key_figures":[],"evidence":[{"claim_key":"","evidence_type":"section","section_id":"<label>","snippet":"","confidence":0.0}]}`

func buildUserPrompt(input Context, maxInputChars int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n\nAbstract: %s\n\n", input.Title, input.Abstract)
	if len(input.Figures) > 0 {
		b.WriteString("Figures and Tables:\n")
		for _, f := range input.Figures {
			fmt.Fprintf(&b, "- [%s] %s\n", f.Label, f.Caption)
		}
		b.WriteString("\n")
	}
	if len(input.Sections) > 0 {
		b.WriteString("Sections:\n")
		for _, s := range input.Sections {
			fmt.Fprintf(&b, "[%s] %s\n%s\n\n", s.Label, s.Heading, s.Text)
		}
	}
	out := b.String()
	if maxInputChars > 0 && len(out) > maxInputChars {
		out = out[:maxInputChars]
	}
	return out
}
