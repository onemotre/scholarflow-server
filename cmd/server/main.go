package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/hibiken/asynq"

	"scholarflow_server/internal/config"
	dbpkg "scholarflow_server/internal/db"
	"scholarflow_server/internal/httpapi"
	"scholarflow_server/internal/jobs"
	"scholarflow_server/internal/migrate"
	"scholarflow_server/internal/papers"
	"scholarflow_server/internal/settings"
	"scholarflow_server/internal/storage"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	if err := migrate.Run(ctx, cfg.DatabaseURL); err != nil {
		log.Fatal(err)
	}

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

	queries := dbpkg.New(pool)
	settingsProvider := settings.NewProvider(settings.NewSQLRepository(queries), 5*time.Second)
	cfg = settingsProvider.Snapshot(ctx) // DB overrides applied; bootstrap values unchanged

	repo := papers.NewSQLRepository(queries)
	enqueuer := jobs.NewEnqueuer(asynqClient, cfg.ReadMaxRetry)
	paperService := papers.NewService(repo, store, enqueuer)
	uploadHandler := httpapi.NewUploadHandler(paperService, cfg.MaxUploadBytes)
	readRepo := papers.NewSQLReadRepository(queries)
	readHandler := httpapi.NewReadHandler(readRepo)
	retryHandler := httpapi.NewRetryHandler(papers.NewRetryService(readRepo, enqueuer))
	figureImageHandler := httpapi.NewFigureImageHandler(readRepo, store)
	harvestHandler := httpapi.NewHarvestHandler(enqueuer)
	adminService := papers.NewAdminService(readRepo, store, enqueuer)
	adminHandler := httpapi.NewAdminHandler(adminService)
	panelHandler := httpapi.NewPanelHandler()
	settingsHandler := httpapi.NewSettingsHandler(settingsProvider)

	log.Printf("starting api on %s", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, httpapi.NewRouter(httpapi.Dependencies{
		UploadHandler:      uploadHandler,
		ReadHandler:        readHandler,
		RetryHandler:       retryHandler,
		FigureImageHandler: figureImageHandler,
		HarvestHandler:     harvestHandler,
		AdminHandler:       adminHandler,
		PanelHandler:       panelHandler,
		SettingsHandler:    settingsHandler,
		WriteAPITokenFn:    func() string { return settingsProvider.String(context.Background(), "WRITE_API_TOKEN") },
	})); err != nil {
		log.Fatal(err)
	}
}
