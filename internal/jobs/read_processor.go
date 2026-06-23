package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

type PaperReaderRunner interface {
	ReadPaper(ctx context.Context, payload ProcessPaperPayload, attempt int32, isFinalAttempt bool) error
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
	retryCount, _ := asynq.GetRetryCount(ctx)
	maxRetry, ok := asynq.GetMaxRetry(ctx)
	attempt := int32(retryCount + 1)
	isFinalAttempt := !ok || retryCount >= maxRetry
	return p.pipeline.ReadPaper(ctx, payload, attempt, isFinalAttempt)
}
