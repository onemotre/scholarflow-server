package reader

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"strings"
)

//go:embed prompts/system.md
var defaultSystemPrompt string

// LoadSystemPrompt returns the contents of the file at path when it is set and
// readable, otherwise the embedded default. It never fails the reader.
func LoadSystemPrompt(path string) string {
	if path == "" {
		return defaultSystemPrompt
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("reader: cannot read OPENAI_SYSTEM_PROMPT_PATH %q (%v); using embedded default", path, err)
		return defaultSystemPrompt
	}
	return string(data)
}

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
