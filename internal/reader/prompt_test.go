package reader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSystemPromptDefault(t *testing.T) {
	got := LoadSystemPrompt("")
	if !strings.Contains(got, "objective research-paper summarizer") {
		t.Fatalf("default prompt missing expected text: %q", got)
	}
}

func TestLoadSystemPromptOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.md")
	if err := os.WriteFile(path, []byte("CUSTOM PROMPT"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadSystemPrompt(path); got != "CUSTOM PROMPT" {
		t.Fatalf("override prompt = %q", got)
	}
}

func TestLoadSystemPromptUnreadableFallsBack(t *testing.T) {
	got := LoadSystemPrompt(filepath.Join(t.TempDir(), "missing.md"))
	if !strings.Contains(got, "objective research-paper summarizer") {
		t.Fatalf("unreadable path should fall back to default, got %q", got)
	}
}
