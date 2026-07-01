package settings

// Kind is the value type of a setting, used by the API/UI to render and by the
// provider to parse and validate.
type Kind string

const (
	KindString Kind = "string"
	KindInt    Kind = "int"
	KindBool   Kind = "bool"
	KindCSV    Kind = "csv"
)

// Apply describes when a setting change takes effect.
type Apply string

const (
	ApplyLive      Apply = "live"      // read per-operation; no restart
	ApplyRestart   Apply = "restart"   // persisted; read at process startup
	ApplyBootstrap Apply = "bootstrap" // env-only, read-only, not DB-overridable
)

// Def declares one setting. Key equals the environment variable name.
type Def struct {
	Key     string
	Group   string
	Kind    Kind
	Secret  bool
	Apply   Apply
	Default string
	Label   string
	Help    string
}

// Registry enumerates every setting. Order is the UI display order.
var Registry = []Def{
	// infra (bootstrap, read-only)
	{Key: "HTTP_ADDR", Group: "infra", Kind: KindString, Apply: ApplyBootstrap, Default: ":8080", Label: "HTTP address", Help: "API listen address. Restart-only infrastructure."},
	{Key: "DATABASE_URL", Group: "infra", Kind: KindString, Secret: true, Apply: ApplyBootstrap, Default: "postgres://scholarflow:scholarflow@localhost:5432/scholarflow?sslmode=disable", Label: "Database URL", Help: "Postgres DSN (contains user:password). Cannot be overridden via the database itself. Value is masked in the API response."},
	{Key: "REDIS_ADDR", Group: "infra", Kind: KindString, Apply: ApplyBootstrap, Default: "localhost:6379", Label: "Redis address", Help: "Asynq/Redis address."},
	{Key: "MINIO_ENDPOINT", Group: "infra", Kind: KindString, Apply: ApplyBootstrap, Default: "localhost:9000", Label: "MinIO endpoint", Help: "Object storage endpoint."},
	{Key: "MINIO_ACCESS_KEY", Group: "infra", Kind: KindString, Apply: ApplyBootstrap, Default: "scholarflow", Label: "MinIO access key", Help: "Object storage access key."},
	{Key: "MINIO_SECRET_KEY", Group: "infra", Kind: KindString, Secret: true, Apply: ApplyBootstrap, Default: "scholarflow-secret", Label: "MinIO secret key", Help: "Object storage secret key."},
	{Key: "MINIO_BUCKET", Group: "infra", Kind: KindString, Apply: ApplyBootstrap, Default: "scholarflow", Label: "MinIO bucket", Help: "Bucket for PDFs and TEI."},
	{Key: "MINIO_USE_SSL", Group: "infra", Kind: KindBool, Apply: ApplyBootstrap, Default: "false", Label: "MinIO use SSL", Help: "Whether to use TLS for MinIO."},

	// auth (live)
	{Key: "WRITE_API_TOKEN", Group: "auth", Kind: KindString, Secret: true, Apply: ApplyLive, Default: "", Label: "Write API token", Help: "Bearer token required for write endpoints. Blank disables auth. Applied live: after changing it the panel must use the new token."},

	// parser (restart in 2A; 2B makes live)
	{Key: "GROBID_URL", Group: "parser", Kind: KindString, Apply: ApplyRestart, Default: "http://localhost:8070", Label: "GROBID URL", Help: "GROBID base URL used by the parse stage."},

	// uploads (restart)
	{Key: "MAX_UPLOAD_BYTES", Group: "uploads", Kind: KindInt, Apply: ApplyRestart, Default: "52428800", Label: "Max upload bytes", Help: "Maximum accepted PDF upload size in bytes."},

	// reader (restart in 2A; 2B makes live)
	{Key: "OPENAI_BASE_URL", Group: "reader", Kind: KindString, Apply: ApplyRestart, Default: "", Label: "OpenAI base URL", Help: "LLM API base URL. Blank disables the reader (jobs stop at parsed)."},
	{Key: "OPENAI_API_KEY", Group: "reader", Kind: KindString, Secret: true, Apply: ApplyRestart, Default: "", Label: "OpenAI API key", Help: "LLM API key. Blank disables the reader."},
	{Key: "OPENAI_MODEL", Group: "reader", Kind: KindString, Apply: ApplyRestart, Default: "gpt-4o-mini", Label: "OpenAI model", Help: "Model name for card generation."},
	{Key: "OPENAI_MAX_INPUT_CHARS", Group: "reader", Kind: KindInt, Apply: ApplyRestart, Default: "48000", Label: "OpenAI max input chars", Help: "Truncate reader input to this many characters."},
	{Key: "OPENAI_TIMEOUT_SECONDS", Group: "reader", Kind: KindInt, Apply: ApplyRestart, Default: "120", Label: "OpenAI timeout (s)", Help: "HTTP timeout for reader calls."},
	{Key: "OPENAI_API_STYLE", Group: "reader", Kind: KindString, Apply: ApplyRestart, Default: "chat", Label: "OpenAI API style", Help: "chat or responses."},
	{Key: "OPENAI_RESPONSE_FORMAT", Group: "reader", Kind: KindString, Apply: ApplyRestart, Default: "json_schema", Label: "OpenAI response format", Help: "json_schema or json_object."},
	{Key: "OPENAI_SYSTEM_PROMPT_PATH", Group: "reader", Kind: KindString, Apply: ApplyRestart, Default: "", Label: "OpenAI system prompt path", Help: "Container path to a system-prompt override file."},

	// jobs (restart)
	{Key: "READ_MAX_RETRY", Group: "jobs", Kind: KindInt, Apply: ApplyRestart, Default: "3", Label: "Read max retry", Help: "Asynq max retries for the read task."},
	{Key: "JOB_FAILED_RETENTION_DAYS", Group: "jobs", Kind: KindInt, Apply: ApplyRestart, Default: "7", Label: "Failed job retention (days)", Help: "Delete failed job rows older than this."},
	{Key: "JOB_CLEANUP_CRON", Group: "jobs", Kind: KindString, Apply: ApplyRestart, Default: "@daily", Label: "Job cleanup cron", Help: "Cron spec for the cleanup schedule."},

	// figures (restart in 2A; 2B makes live)
	{Key: "FIGURE_EXTRACT_ENABLED", Group: "figures", Kind: KindBool, Apply: ApplyRestart, Default: "true", Label: "Figure extraction enabled", Help: "Whether to crop figure images during parse."},
	{Key: "FIGURE_EXTRACT_DPI", Group: "figures", Kind: KindInt, Apply: ApplyRestart, Default: "150", Label: "Figure DPI", Help: "Render DPI for figure crops."},
	{Key: "FIGURE_EXTRACT_PADDING_PCT", Group: "figures", Kind: KindInt, Apply: ApplyRestart, Default: "2", Label: "Figure padding (%)", Help: "Padding percentage around figure bounding boxes."},
	{Key: "FIGURE_EXTRACT_MAX_DIM", Group: "figures", Kind: KindInt, Apply: ApplyRestart, Default: "2000", Label: "Figure max dimension", Help: "Max pixel dimension for figure crops."},

	// harvest (restart)
	{Key: "ARXIV_HARVEST_ENABLED", Group: "harvest", Kind: KindBool, Apply: ApplyRestart, Default: "false", Label: "arXiv harvest enabled", Help: "Enable the scheduled arXiv harvest cron."},
	{Key: "ARXIV_HARVEST_CATEGORIES", Group: "harvest", Kind: KindCSV, Apply: ApplyRestart, Default: "", Label: "arXiv categories", Help: "Comma-separated arXiv categories to harvest."},
	{Key: "ARXIV_HARVEST_CRON", Group: "harvest", Kind: KindString, Apply: ApplyRestart, Default: "@daily", Label: "arXiv harvest cron", Help: "Cron spec for the harvest schedule."},
	{Key: "ARXIV_HARVEST_MAX_RESULTS", Group: "harvest", Kind: KindInt, Apply: ApplyRestart, Default: "50", Label: "arXiv max results", Help: "Max results per harvest run."},
	{Key: "ARXIV_API_BASE_URL", Group: "harvest", Kind: KindString, Apply: ApplyRestart, Default: "http://export.arxiv.org/api/query", Label: "arXiv API base URL", Help: "arXiv API endpoint."},
	{Key: "ARXIV_REQUEST_DELAY_SECONDS", Group: "harvest", Kind: KindInt, Apply: ApplyRestart, Default: "3", Label: "arXiv request delay (s)", Help: "Delay between arXiv requests."},
	{Key: "ARXIV_HARVEST_TIMEZONE", Group: "harvest", Kind: KindString, Apply: ApplyRestart, Default: "", Label: "arXiv harvest timezone", Help: "IANA timezone for harvest cron; blank uses host local."},
}

// ByKey returns the Def for key, if present.
func ByKey(key string) (Def, bool) {
	for _, d := range Registry {
		if d.Key == key {
			return d, true
		}
	}
	return Def{}, false
}
