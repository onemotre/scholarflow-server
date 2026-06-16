package main

import (
	"context"
	"log"

	"github.com/hibiken/asynq"

	"scholarflow_server/internal/config"
	dbpkg "scholarflow_server/internal/db"
	"scholarflow_server/internal/jobs"
	"scholarflow_server/internal/parser"
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
	pipeline := jobs.NewPipeline(repo, store, parser.NewGROBIDParser(cfg.GROBIDURL))

	server := asynq.NewServer(asynq.RedisClientOpt{Addr: cfg.RedisAddr}, asynq.Config{Concurrency: 2})
	mux := asynq.NewServeMux()
	jobs.NewProcessor(pipeline).Register(mux)
	log.Printf("starting worker redis=%s grobid=%s", cfg.RedisAddr, cfg.GROBIDURL)
	if err := server.Run(mux); err != nil {
		log.Fatal(err)
	}
}
