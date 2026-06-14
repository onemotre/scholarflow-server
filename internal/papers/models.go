package papers

import "github.com/google/uuid"

type UploadResult struct {
	PaperID uuid.UUID `json:"paper_id"`
	JobID   uuid.UUID `json:"job_id"`
}
