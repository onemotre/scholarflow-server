package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

type Processor struct{}

func NewProcessor() *Processor {
	return &Processor{}
}

func (p *Processor) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeProcessPaper, p.HandleProcessPaper)
}

func (p *Processor) HandleProcessPaper(ctx context.Context, task *asynq.Task) error {
	var payload ProcessPaperPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("decode process paper payload: %w", err)
	}
	return nil
}
