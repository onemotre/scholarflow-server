package sources

import (
	"context"
	"time"
)

// Entry is a source-agnostic description of a paper discovered by a Source.
type Entry struct {
	SourceID        string // stable id within the source, e.g. normalized arxiv id "2301.00001"
	Title           string
	Abstract        string
	DOI             string
	PrimaryCategory string
	Authors         []string
	Published       time.Time
	PDFURL          string
}

// Source is a pluggable provider of recently-published papers. arXiv is the
// first implementation; others (bioRxiv, Semantic Scholar) can be added without
// changing the harvest machinery.
type Source interface {
	// Name identifies the source and becomes papers.source_type.
	Name() string
	// FetchRecent returns up to maxResults most-recently-submitted entries for a category.
	FetchRecent(ctx context.Context, category string, maxResults int) ([]Entry, error)
}
