package main

import (
	"context"
	"log"
	"time"

	"github.com/hibiken/asynq"

	"scholarflow_server/internal/config"
	dbpkg "scholarflow_server/internal/db"
	"scholarflow_server/internal/jobs"
	"scholarflow_server/internal/parser"
	"scholarflow_server/internal/reader"
	"scholarflow_server/internal/storage"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	pool, err := dbpkg.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	store, err := storage.NewMinIOStore(cfg.MinIOEndpoint, cfg.MinIOAccessKey, cfg.MinIOSecretKey, cfg.MinIOBucket, cfg.MinIOUseSSL)
	if err != nil {
		log.Fatal(err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		log.Fatal(err)
	}

	repo := jobs.NewSQLRepository(dbpkg.New(pool))
	redisOpt := asynq.RedisClientOpt{Addr: cfg.RedisAddr}

	mux := asynq.NewServeMux()

	readerConfigured := cfg.OpenAIBaseURL != "" && cfg.OpenAIAPIKey != ""
	var readEnqueuer jobs.ReadEnqueuer
	if readerConfigured {
		client := asynq.NewClient(redisOpt)
		defer client.Close()
		enqueuer := jobs.NewEnqueuer(client)
		readEnqueuer = enqueuer

		rdr := reader.NewOpenAIReader(reader.OpenAIConfig{
			BaseURL:        cfg.OpenAIBaseURL,
			APIKey:         cfg.OpenAIAPIKey,
			Model:          cfg.OpenAIModel,
			APIStyle:       cfg.OpenAIAPIStyle,
			ResponseFormat: cfg.OpenAIResponseFormat,
			SystemPrompt:   reader.LoadSystemPrompt(cfg.OpenAISystemPromptPath),
			MaxInputChars:  cfg.OpenAIMaxInputChars,
			Timeout:        time.Duration(cfg.OpenAITimeoutSeconds) * time.Second,
		})
		readPipeline := jobs.NewReadPipeline(repo, rdr, cfg.OpenAIModel)
		jobs.NewReadProcessor(readPipeline).Register(mux)
		log.Printf("reader enabled model=%s base=%s style=%s format=%s", cfg.OpenAIModel, cfg.OpenAIBaseURL, cfg.OpenAIAPIStyle, cfg.OpenAIResponseFormat)
	} else {
		log.Printf("reader disabled (OPENAI_BASE_URL / OPENAI_API_KEY not set); jobs stop at parsed")
	}

	pipeline := jobs.NewPipeline(repo, store, parser.NewGROBIDParser(cfg.GROBIDURL), readEnqueuer)
	jobs.NewProcessor(pipeline).Register(mux)

	server := asynq.NewServer(redisOpt, asynq.Config{Concurrency: 2})
	log.Printf("starting worker redis=%s grobid=%s", cfg.RedisAddr, cfg.GROBIDURL)
	if err := server.Run(mux); err != nil {
		log.Fatal(err)
	}
}
