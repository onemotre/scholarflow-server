package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// HarvestRunner is the behavior the processor adapts to an asynq handler.
// categories overrides the configured categories when non-empty.
type HarvestRunner interface {
	Harvest(ctx context.Context, categories []string) error
}

type HarvestProcessor struct {
	pipeline HarvestRunner
}

func NewHarvestProcessor(pipeline HarvestRunner) *HarvestProcessor {
	return &HarvestProcessor{pipeline: pipeline}
}

func (p *HarvestProcessor) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(TypeHarvestArxiv, p.HandleHarvest)
}

func (p *HarvestProcessor) HandleHarvest(ctx context.Context, task *asynq.Task) error {
	var payload HarvestArxivPayload
	if len(task.Payload()) > 0 {
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("decode harvest payload: %w", err)
		}
	}
	return p.pipeline.Harvest(ctx, payload.Categories)
}
