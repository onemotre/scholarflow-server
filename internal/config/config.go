package config

import (
	"os"
	"strconv"
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
