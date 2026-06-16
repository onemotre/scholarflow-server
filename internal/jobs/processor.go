package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

type PaperProcessor interface {
	ProcessPaper(ctx context.Context, payload ProcessPaperPayload) error
}

type Processor struct {
	pipeline PaperProcessor
}

func NewProcessor(pipeline PaperProcessor) *Processor {
	return &Processor{pipeline: pipeline}
}

func (p *Processor) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeProcessPaper, p.HandleProcessPaper)
}

func (p *Processor) HandleProcessPaper(ctx context.Context, task *asynq.Task) error {
	var payload ProcessPaperPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode process paper payload: %w", err)
	}
	if p.pipeline == nil {
		return fmt.Errorf("paper processing pipeline is not configured")
	}
	return p.pipeline.ProcessPaper(ctx, payload)
}
