package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr                string
	DatabaseURL             string
	RedisAddr               string
	MinIOEndpoint           string
	MinIOAccessKey          string
	MinIOSecretKey          string
	MinIOBucket             string
	MinIOUseSSL             bool
	GROBIDURL               string
	MaxUploadBytes          int64
	WriteAPIToken           string
	OpenAIBaseURL           string
	OpenAIAPIKey            string
	OpenAIModel             string
	OpenAIMaxInputChars     int
	OpenAITimeoutSeconds    int
	OpenAIAPIStyle          string
	OpenAIResponseFormat    string
	OpenAISystemPromptPath  string
	ReadMaxRetry            int
	JobFailedRetentionDays  int
	JobCleanupCron          string
	FigureExtractEnabled    bool
	FigureExtractDPI        int
	FigureExtractPaddingPct int
	FigureExtractMaxDim     int

	ArxivHarvestEnabled      bool
	ArxivHarvestCategories   []string
	ArxivHarvestCron         string
	ArxivHarvestMaxResults   int
	ArxivAPIBaseURL          string
	ArxivRequestDelaySeconds int
	ArxivHarvestTimezone     string
}

func Load() Config {
	return Config{
		HTTPAddr:                envString("HTTP_ADDR", ":8080"),
		DatabaseURL:             envString("DATABASE_URL", "postgres://scholarflow:scholarflow@localhost:5432/scholarflow?sslmode=disable"),
		RedisAddr:               envString("REDIS_ADDR", "localhost:6379"),
		MinIOEndpoint:           envString("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:          envString("MINIO_ACCESS_KEY", "scholarflow"),
		MinIOSecretKey:          envString("MINIO_SECRET_KEY", "scholarflow-secret"),
		MinIOBucket:             envString("MINIO_BUCKET", "scholarflow"),
		MinIOUseSSL:             envBool("MINIO_USE_SSL", false),
		GROBIDURL:               envString("GROBID_URL", "http://localhost:8070"),
		MaxUploadBytes:          envInt64("MAX_UPLOAD_BYTES", 50*1024*1024),
		WriteAPIToken:           envString("WRITE_API_TOKEN", ""),
		OpenAIBaseURL:           envString("OPENAI_BASE_URL", ""),
		OpenAIAPIKey:            envString("OPENAI_API_KEY", ""),
		OpenAIModel:             envString("OPENAI_MODEL", "gpt-4o-mini"),
		OpenAIMaxInputChars:     int(envInt64("OPENAI_MAX_INPUT_CHARS", 48000)),
		OpenAITimeoutSeconds:    int(envInt64("OPENAI_TIMEOUT_SECONDS", 120)),
		OpenAIAPIStyle:          envString("OPENAI_API_STYLE", "chat"),
		OpenAIResponseFormat:    envString("OPENAI_RESPONSE_FORMAT", "json_schema"),
		OpenAISystemPromptPath:  envString("OPENAI_SYSTEM_PROMPT_PATH", ""),
		ReadMaxRetry:            int(envInt64("READ_MAX_RETRY", 3)),
		JobFailedRetentionDays:  int(envInt64("JOB_FAILED_RETENTION_DAYS", 7)),
		JobCleanupCron:          envString("JOB_CLEANUP_CRON", "@daily"),
		FigureExtractEnabled:    envBool("FIGURE_EXTRACT_ENABLED", true),
		FigureExtractDPI:        int(envInt64("FIGURE_EXTRACT_DPI", 150)),
		FigureExtractPaddingPct: int(envInt64("FIGURE_EXTRACT_PADDING_PCT", 2)),
		FigureExtractMaxDim:     int(envInt64("FIGURE_EXTRACT_MAX_DIM", 2000)),

		ArxivHarvestEnabled:      envBool("ARXIV_HARVEST_ENABLED", false),
		ArxivHarvestCategories:   envCSV("ARXIV_HARVEST_CATEGORIES"),
		ArxivHarvestCron:         envString("ARXIV_HARVEST_CRON", "@daily"),
		ArxivHarvestMaxResults:   int(envInt64("ARXIV_HARVEST_MAX_RESULTS", 50)),
		ArxivAPIBaseURL:          envString("ARXIV_API_BASE_URL", "http://export.arxiv.org/api/query"),
		ArxivRequestDelaySeconds: int(envInt64("ARXIV_REQUEST_DELAY_SECONDS", 3)),
		ArxivHarvestTimezone:     envString("ARXIV_HARVEST_TIMEZONE", ""),
	}
}

func envString(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

// envCSV parses a comma-separated env var into a trimmed, non-empty slice.
// ResolveLocation returns the time.Location for scheduling. An empty tz uses the
// deployment host's local timezone (TZ env / time.Local); otherwise tz is looked
// up in the IANA database (e.g. "Asia/Shanghai").
func ResolveLocation(tz string) (*time.Location, error) {
	if tz == "" {
		return time.Local, nil
	}
	return time.LoadLocation(tz)
}

func envCSV(key string) []string {
	value := os.Getenv(key)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
