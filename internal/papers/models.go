package papers

import "github.com/google/uuid"

type UploadResult struct {
	PaperID uuid.UUID `json:"paper_id"`
	JobID   uuid.UUID `json:"job_id"`
}

// SourceInfo describes where a paper came from and any metadata known at
// ingestion time (before GROBID parse). SourceID is empty for local uploads.
type SourceInfo struct {
	SourceType      string
	SourceID        string
	Filename        string
	Title           string
	Abstract        string
	DOI             string
	Year            int32
	PrimaryCategory string
}
