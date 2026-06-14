package main

import (
	"log"

	"github.com/hibiken/asynq"

	"scholarflow_server/internal/config"
	"scholarflow_server/internal/jobs"
)

func main() {
	cfg := config.Load()
	server := asynq.NewServer(asynq.RedisClientOpt{Addr: cfg.RedisAddr}, asynq.Config{Concurrency: 2})
	mux := asynq.NewServeMux()
	jobs.NewProcessor().Register(mux)
	log.Printf("starting worker redis=%s grobid=%s", cfg.RedisAddr, cfg.GROBIDURL)
	if err := server.Run(mux); err != nil {
		log.Fatal(err)
	}
}
