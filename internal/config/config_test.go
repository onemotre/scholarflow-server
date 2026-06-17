package config

import "testing"

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
