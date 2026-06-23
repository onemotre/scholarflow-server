package jobs

import (
	"encoding/json"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

const TypeProcessPaper = "paper:process"
const TypeReadPaper = "paper:read"
const TypeCleanupJobs = "jobs:cleanup"

type ProcessPaperPayload struct {
	PaperID uuid.UUID `json:"paper_id"`
	JobID   uuid.UUID `json:"job_id"`
}

func NewProcessPaperTask(paperID uuid.UUID, jobID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(ProcessPaperPayload{PaperID: paperID, JobID: jobID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeProcessPaper, payload), nil
}

func NewReadPaperTask(paperID uuid.UUID, jobID uuid.UUID) (*asynq.Task, error) {
	payload, err := json.Marshal(ProcessPaperPayload{PaperID: paperID, JobID: jobID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeReadPaper, payload), nil
}

func NewCleanupJobsTask() (*asynq.Task, error) {
	return asynq.NewTask(TypeCleanupJobs, nil), nil
}
