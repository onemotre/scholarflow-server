package reader

import (
	"strings"
	"testing"
)

func TestBuildUserPromptIncludesFigures(t *testing.T) {
	out := buildUserPrompt(Context{
		Title:    "T",
		Abstract: "A",
		Figures:  []Figure{{Label: "Figure 1", Kind: "figure", Caption: "A plot of results."}},
		Sections: []Section{{Label: "1", Heading: "Intro", Text: "body"}},
	}, 48000)
	if !strings.Contains(out, "Figures and Tables:") {
		t.Fatalf("missing figures header: %s", out)
	}
	if !strings.Contains(out, "[Figure 1] A plot of results.") {
		t.Fatalf("missing figure caption line: %s", out)
	}
}
