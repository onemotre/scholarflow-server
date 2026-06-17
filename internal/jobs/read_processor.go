package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

type PaperReaderRunner interface {
	ReadPaper(ctx context.Context, payload ProcessPaperPayload) error
}

type ReadProcessor struct {
	pipeline PaperReaderRunner
}

func NewReadProcessor(pipeline PaperReaderRunner) *ReadProcessor {
	return &ReadProcessor{pipeline: pipeline}
}

func (p *ReadProcessor) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeReadPaper, p.HandleReadPaper)
}

func (p *ReadProcessor) HandleReadPaper(ctx context.Context, task *asynq.Task) error {
	var payload ProcessPaperPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode read paper payload: %w", err)
	}
	if p.pipeline == nil {
		return fmt.Errorf("read pipeline is not configured")
	}
	return p.pipeline.ReadPaper(ctx, payload)
}
