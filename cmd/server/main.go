package main

import (
	"context"
	"log"
	"net/http"

	"github.com/hibiken/asynq"

	"scholarflow_server/internal/config"
	dbpkg "scholarflow_server/internal/db"
	"scholarflow_server/internal/httpapi"
	"scholarflow_server/internal/jobs"
	"scholarflow_server/internal/papers"
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

	asynqClient := asynq.NewClient(asynq.RedisClientOpt{Addr: cfg.RedisAddr})
	defer asynqClient.Close()

	repo := papers.NewSQLRepository(dbpkg.New(pool))
	enqueuer := jobs.NewEnqueuer(asynqClient)
	paperService := papers.NewService(repo, store, enqueuer)
	uploadHandler := httpapi.NewUploadHandler(paperService, cfg.MaxUploadBytes)

	log.Printf("starting api on %s", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, httpapi.NewRouter(httpapi.Dependencies{UploadHandler: uploadHandler})); err != nil {
		log.Fatal(err)
	}
}
