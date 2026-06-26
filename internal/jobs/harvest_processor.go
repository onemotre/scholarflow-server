package jobs

import (
	"context"

	"github.com/hibiken/asynq"
)

// HarvestRunner is the behavior the processor adapts to an asynq handler.
type HarvestRunner interface {
	Harvest(ctx context.Context) error
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

func (p *HarvestProcessor) HandleHarvest(ctx context.Context, _ *asynq.Task) error {
	return p.pipeline.Harvest(ctx)
}
