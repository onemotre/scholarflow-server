package jobs

import (
	"context"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type Enqueuer struct {
	client       *asynq.Client
	readMaxRetry int
}

func NewEnqueuer(client *asynq.Client, readMaxRetry int) *Enqueuer {
	return &Enqueuer{client: client, readMaxRetry: readMaxRetry}
}

func (e *Enqueuer) EnqueuePaperProcessing(ctx context.Context, paperID uuid.UUID, jobID uuid.UUID) (string, error) {
	task, err := NewProcessPaperTask(paperID, jobID)
	if err != nil {
		return "", err
	}
	info, err := e.client.EnqueueContext(ctx, task)
	if err != nil {
		return "", err
	}
	return info.ID, nil
}

func (e *Enqueuer) EnqueuePaperRead(ctx context.Context, paperID uuid.UUID, jobID uuid.UUID) (string, error) {
	task, err := NewReadPaperTask(paperID, jobID)
	if err != nil {
		return "", err
	}
	info, err := e.client.EnqueueContext(ctx, task, asynq.MaxRetry(e.readMaxRetry))
	if err != nil {
		return "", err
	}
	return info.ID, nil
}
