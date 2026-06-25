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

func TestDefaultPromptDescribesV3(t *testing.T) {
	got := LoadSystemPrompt("")
	for _, marker := range []string{"introduction", "related_work", "methodology", "self_only", "modules"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("default prompt missing v3 marker %q", marker)
		}
	}
	if strings.Contains(got, "\"background\"") {
		t.Fatalf("default prompt still references removed 2.0 field \"background\"")
	}
}

func TestChinesePromptDescribesV3(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("prompts", "system_zh.md"))
	if err != nil {
		t.Fatalf("read system_zh.md: %v", err)
	}
	got := string(data)
	for _, marker := range []string{"introduction", "related_work", "methodology", "self_only", "modules", "Simplified Chinese"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("system_zh.md missing marker %q", marker)
		}
	}
}
