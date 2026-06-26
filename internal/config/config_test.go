package config

import (
	"testing"
	"time"
)

func TestLoadArxivTimezone(t *testing.T) {
	t.Setenv("ARXIV_HARVEST_TIMEZONE", "")
	if cfg := Load(); cfg.ArxivHarvestTimezone != "" {
		t.Fatalf("timezone default = %q, want empty", cfg.ArxivHarvestTimezone)
	}
	t.Setenv("ARXIV_HARVEST_TIMEZONE", "Asia/Shanghai")
	if cfg := Load(); cfg.ArxivHarvestTimezone != "Asia/Shanghai" {
		t.Fatalf("timezone = %q, want Asia/Shanghai", cfg.ArxivHarvestTimezone)
	}
}

func TestResolveLocation(t *testing.T) {
	loc, err := ResolveLocation("")
	if err != nil {
		t.Fatalf("empty tz error: %v", err)
	}
	if loc != time.Local {
		t.Fatalf("empty tz = %v, want time.Local", loc)
	}

	loc, err = ResolveLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("Asia/Shanghai error: %v", err)
	}
	if loc.String() != "Asia/Shanghai" {
		t.Fatalf("loc = %q, want Asia/Shanghai", loc.String())
	}

	if _, err := ResolveLocation("Not/AZone"); err == nil {
		t.Fatal("invalid tz returned nil error, want error")
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	cfg := Load()

	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.MaxUploadBytes != 50*1024*1024 {
		t.Fatalf("MaxUploadBytes = %d, want %d", cfg.MaxUploadBytes, 50*1024*1024)
	}
}

func TestLoadEnvironment(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("MAX_UPLOAD_BYTES", "12345")
	t.Setenv("MINIO_USE_SSL", "true")

	cfg := Load()

	if cfg.HTTPAddr != ":9090" {
		t.Fatalf("HTTPAddr = %q, want :9090", cfg.HTTPAddr)
	}
	if cfg.MaxUploadBytes != 12345 {
		t.Fatalf("MaxUploadBytes = %d, want 12345", cfg.MaxUploadBytes)
	}
	if !cfg.MinIOUseSSL {
		t.Fatal("MinIOUseSSL = false, want true")
	}
}

func TestLoadOpenAIDefaults(t *testing.T) {
	for _, k := range []string{"OPENAI_BASE_URL", "OPENAI_API_KEY", "OPENAI_MODEL", "OPENAI_MAX_INPUT_CHARS", "OPENAI_TIMEOUT_SECONDS"} {
		t.Setenv(k, "")
	}
	cfg := Load()
	if cfg.OpenAIBaseURL != "" || cfg.OpenAIAPIKey != "" {
		t.Fatalf("base url/key should default blank, got %q/%q", cfg.OpenAIBaseURL, cfg.OpenAIAPIKey)
	}
	if cfg.OpenAIModel != "gpt-4o-mini" {
		t.Fatalf("model default = %q", cfg.OpenAIModel)
	}
	if cfg.OpenAIMaxInputChars != 48000 {
		t.Fatalf("max input chars = %d", cfg.OpenAIMaxInputChars)
	}
	if cfg.OpenAITimeoutSeconds != 120 {
		t.Fatalf("timeout = %d", cfg.OpenAITimeoutSeconds)
	}
}

func TestLoadArxivDefaults(t *testing.T) {
	for _, k := range []string{"ARXIV_HARVEST_ENABLED", "ARXIV_HARVEST_CATEGORIES", "ARXIV_HARVEST_CRON", "ARXIV_HARVEST_MAX_RESULTS", "ARXIV_API_BASE_URL", "ARXIV_REQUEST_DELAY_SECONDS"} {
		t.Setenv(k, "")
	}
	cfg := Load()
	if cfg.ArxivHarvestEnabled {
		t.Fatal("ArxivHarvestEnabled should default false")
	}
	if len(cfg.ArxivHarvestCategories) != 0 {
		t.Fatalf("categories = %#v, want empty", cfg.ArxivHarvestCategories)
	}
	if cfg.ArxivHarvestCron != "@daily" {
		t.Fatalf("cron = %q, want @daily", cfg.ArxivHarvestCron)
	}
	if cfg.ArxivHarvestMaxResults != 50 {
		t.Fatalf("max results = %d, want 50", cfg.ArxivHarvestMaxResults)
	}
	if cfg.ArxivAPIBaseURL != "http://export.arxiv.org/api/query" {
		t.Fatalf("base url = %q", cfg.ArxivAPIBaseURL)
	}
	if cfg.ArxivRequestDelaySeconds != 3 {
		t.Fatalf("delay = %d, want 3", cfg.ArxivRequestDelaySeconds)
	}
}

func TestLoadArxivCategoriesParsed(t *testing.T) {
	t.Setenv("ARXIV_HARVEST_ENABLED", "true")
	t.Setenv("ARXIV_HARVEST_CATEGORIES", "cs.CL, cs.AI ,cs.LG")
	cfg := Load()
	if !cfg.ArxivHarvestEnabled {
		t.Fatal("ArxivHarvestEnabled = false, want true")
	}
	want := []string{"cs.CL", "cs.AI", "cs.LG"}
	if len(cfg.ArxivHarvestCategories) != 3 {
		t.Fatalf("categories = %#v, want %v", cfg.ArxivHarvestCategories, want)
	}
	for i, c := range want {
		if cfg.ArxivHarvestCategories[i] != c {
			t.Fatalf("categories[%d] = %q, want %q (trimmed)", i, cfg.ArxivHarvestCategories[i], c)
		}
	}
}

func TestEnvCSVEmptyDrop(t *testing.T) {
	t.Setenv("ARXIV_HARVEST_CATEGORIES", "cs.CL,,cs.AI ,")
	cfg := Load()
	want := []string{"cs.CL", "cs.AI"}
	if len(cfg.ArxivHarvestCategories) != len(want) {
		t.Fatalf("categories = %#v, want %v", cfg.ArxivHarvestCategories, want)
	}
	for i, c := range want {
		if cfg.ArxivHarvestCategories[i] != c {
			t.Fatalf("categories[%d] = %q, want %q", i, cfg.ArxivHarvestCategories[i], c)
		}
	}
}
