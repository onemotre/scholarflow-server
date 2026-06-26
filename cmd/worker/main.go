package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"scholarflow_server/internal/arxiv"
	"scholarflow_server/internal/config"
	dbpkg "scholarflow_server/internal/db"
	"scholarflow_server/internal/figures"
	"scholarflow_server/internal/jobs"
	"scholarflow_server/internal/migrate"
	"scholarflow_server/internal/papers"
	"scholarflow_server/internal/parser"
	"scholarflow_server/internal/reader"
	"scholarflow_server/internal/sources"
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

	repo := jobs.NewSQLRepository(dbpkg.New(pool))
	redisOpt := asynq.RedisClientOpt{Addr: cfg.RedisAddr}

	mux := asynq.NewServeMux()

	readerConfigured := cfg.OpenAIBaseURL != "" && cfg.OpenAIAPIKey != ""
	var readEnqueuer jobs.ReadEnqueuer
	if readerConfigured {
		client := asynq.NewClient(redisOpt)
		defer client.Close()
		enqueuer := jobs.NewEnqueuer(client, cfg.ReadMaxRetry)
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

	var cropper figures.Cropper
	if cfg.FigureExtractEnabled {
		cropper = figures.NewPopplerCropper(float64(cfg.FigureExtractPaddingPct), cfg.FigureExtractMaxDim, os.TempDir())
	}
	pipeline := jobs.NewPipeline(repo, store, parser.NewGROBIDParser(cfg.GROBIDURL), readEnqueuer, cropper, cfg.FigureExtractDPI)
	jobs.NewProcessor(pipeline).Register(mux)

	jobs.NewCleanupProcessor(repo, cfg.JobFailedRetentionDays).Register(mux)

	// arXiv daily harvest (opt-in). Registers an arxiv:harvest processor and a
	// scheduler cron; downloads new papers and feeds them into the normal pipeline.
	if cfg.ArxivHarvestEnabled && len(cfg.ArxivHarvestCategories) > 0 {
		harvestClient := asynq.NewClient(redisOpt)
		defer harvestClient.Close()
		processEnqueuer := jobs.NewEnqueuer(harvestClient, cfg.ReadMaxRetry)
		papersRepo := papers.NewSQLRepository(dbpkg.New(pool))
		ingestService := papers.NewService(papersRepo, store, processEnqueuer)

		arxivClient := arxiv.NewClient(cfg.ArxivAPIBaseURL, time.Duration(cfg.ArxivRequestDelaySeconds)*time.Second)
		fetcher := jobs.NewHTTPPDFFetcher(120*time.Second, cfg.MaxUploadBytes)
		harvestPipeline := jobs.NewHarvestPipeline(
			[]sources.Source{arxivClient},
			cfg.ArxivHarvestCategories,
			cfg.ArxivHarvestMaxResults,
			time.Duration(cfg.ArxivRequestDelaySeconds)*time.Second,
			ingestService,
			fetcher,
		)
		jobs.NewHarvestProcessor(harvestPipeline).Register(mux)
		log.Printf("arxiv harvest enabled categories=%s cron=%s max_results=%d",
			strings.Join(cfg.ArxivHarvestCategories, ","), cfg.ArxivHarvestCron, cfg.ArxivHarvestMaxResults)
	}

	server := asynq.NewServer(redisOpt, asynq.Config{Concurrency: 2})

	scheduler := asynq.NewScheduler(redisOpt, &asynq.SchedulerOpts{})
	cleanupTask, err := jobs.NewCleanupJobsTask()
	if err != nil {
		log.Fatal(err)
	}
	if _, err := scheduler.Register(cfg.JobCleanupCron, cleanupTask); err != nil {
		log.Fatal(err)
	}
	if cfg.ArxivHarvestEnabled && len(cfg.ArxivHarvestCategories) > 0 {
		harvestTask, err := jobs.NewHarvestArxivTask()
		if err != nil {
			log.Fatal(err)
		}
		if _, err := scheduler.Register(cfg.ArxivHarvestCron, harvestTask); err != nil {
			log.Fatal(err)
		}
	}
	if err := scheduler.Start(); err != nil {
		log.Fatal(err)
	}
	defer scheduler.Shutdown()

	log.Printf("starting worker redis=%s grobid=%s cleanup_cron=%s", cfg.RedisAddr, cfg.GROBIDURL, cfg.JobCleanupCron)
	if err := server.Run(mux); err != nil {
		log.Fatal(err)
	}
}
