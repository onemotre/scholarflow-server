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

// Getter looks up a raw setting value by key. ok is false when the key is unset.
type Getter func(key string) (string, bool)

// Load reads configuration from the process environment.
func Load() Config { return Build(os.LookupEnv) }

// Build assembles a Config by resolving each key through get, applying the same
// defaults and parsing rules as the environment loader. A value is used only
// when get returns ok and a non-empty string; otherwise the fallback applies.
func Build(get Getter) Config {
	return Config{
		HTTPAddr:                 str(get, "HTTP_ADDR", ":8080"),
		DatabaseURL:              str(get, "DATABASE_URL", "postgres://scholarflow:scholarflow@localhost:5432/scholarflow?sslmode=disable"),
		RedisAddr:                str(get, "REDIS_ADDR", "localhost:6379"),
		MinIOEndpoint:            str(get, "MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:           str(get, "MINIO_ACCESS_KEY", "scholarflow"),
		MinIOSecretKey:           str(get, "MINIO_SECRET_KEY", "scholarflow-secret"),
		MinIOBucket:              str(get, "MINIO_BUCKET", "scholarflow"),
		MinIOUseSSL:              boolFrom(get, "MINIO_USE_SSL", false),
		GROBIDURL:                str(get, "GROBID_URL", "http://localhost:8070"),
		MaxUploadBytes:           int64From(get, "MAX_UPLOAD_BYTES", 50*1024*1024),
		WriteAPIToken:            str(get, "WRITE_API_TOKEN", ""),
		OpenAIBaseURL:            str(get, "OPENAI_BASE_URL", ""),
		OpenAIAPIKey:             str(get, "OPENAI_API_KEY", ""),
		OpenAIModel:              str(get, "OPENAI_MODEL", "gpt-4o-mini"),
		OpenAIMaxInputChars:      int(int64From(get, "OPENAI_MAX_INPUT_CHARS", 48000)),
		OpenAITimeoutSeconds:     int(int64From(get, "OPENAI_TIMEOUT_SECONDS", 120)),
		OpenAIAPIStyle:           str(get, "OPENAI_API_STYLE", "chat"),
		OpenAIResponseFormat:     str(get, "OPENAI_RESPONSE_FORMAT", "json_schema"),
		OpenAISystemPromptPath:   str(get, "OPENAI_SYSTEM_PROMPT_PATH", ""),
		ReadMaxRetry:             int(int64From(get, "READ_MAX_RETRY", 3)),
		JobFailedRetentionDays:   int(int64From(get, "JOB_FAILED_RETENTION_DAYS", 7)),
		JobCleanupCron:           str(get, "JOB_CLEANUP_CRON", "@daily"),
		FigureExtractEnabled:     boolFrom(get, "FIGURE_EXTRACT_ENABLED", true),
		FigureExtractDPI:         int(int64From(get, "FIGURE_EXTRACT_DPI", 150)),
		FigureExtractPaddingPct:  int(int64From(get, "FIGURE_EXTRACT_PADDING_PCT", 2)),
		FigureExtractMaxDim:      int(int64From(get, "FIGURE_EXTRACT_MAX_DIM", 2000)),
		ArxivHarvestEnabled:      boolFrom(get, "ARXIV_HARVEST_ENABLED", false),
		ArxivHarvestCategories:   csvFrom(get, "ARXIV_HARVEST_CATEGORIES"),
		ArxivHarvestCron:         str(get, "ARXIV_HARVEST_CRON", "@daily"),
		ArxivHarvestMaxResults:   int(int64From(get, "ARXIV_HARVEST_MAX_RESULTS", 50)),
		ArxivAPIBaseURL:          str(get, "ARXIV_API_BASE_URL", "http://export.arxiv.org/api/query"),
		ArxivRequestDelaySeconds: int(int64From(get, "ARXIV_REQUEST_DELAY_SECONDS", 3)),
		ArxivHarvestTimezone:     str(get, "ARXIV_HARVEST_TIMEZONE", ""),
	}
}

func str(get Getter, key, fallback string) string {
	if v, ok := get(key); ok && v != "" {
		return v
	}
	return fallback
}

func boolFrom(get Getter, key string, fallback bool) bool {
	v, ok := get(key)
	if !ok || v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func int64From(get Getter, key string, fallback int64) int64 {
	v, ok := get(key)
	if !ok || v == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func csvFrom(get Getter, key string) []string {
	v, ok := get(key)
	if !ok || v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// ResolveLocation returns the time.Location for scheduling. An empty tz uses the
// deployment host's local timezone (TZ env / time.Local); otherwise tz is looked
// up in the IANA database (e.g. "Asia/Shanghai").
func ResolveLocation(tz string) (*time.Location, error) {
	if tz == "" {
		return time.Local, nil
	}
	return time.LoadLocation(tz)
}
