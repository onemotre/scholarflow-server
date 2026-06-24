package papers

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"scholarflow_server/internal/db"
)

// ErrNotFound is returned by read methods when the requested resource does not exist.
var ErrNotFound = errors.New("not found")

type JobStatus struct {
	JobID        uuid.UUID  `json:"job_id"`
	PaperID      uuid.UUID  `json:"paper_id"`
	Status       string     `json:"status"`
	AttemptCount int32      `json:"attempt_count"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	CreatedAt    *time.Time `json:"created_at,omitempty"`
	UpdatedAt    *time.Time `json:"updated_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

type PaperDetail struct {
	PaperID          uuid.UUID       `json:"paper_id"`
	SourceType       string          `json:"source_type"`
	Status           string          `json:"status"`
	Title            *string         `json:"title,omitempty"`
	Abstract         *string         `json:"abstract,omitempty"`
	DOI              *string         `json:"doi,omitempty"`
	PublicationYear  *int32          `json:"publication_year,omitempty"`
	UploadedFilename string          `json:"uploaded_filename"`
	CreatedAt        *time.Time      `json:"created_at,omitempty"`
	UpdatedAt        *time.Time      `json:"updated_at,omitempty"`
	Authors          []AuthorDTO     `json:"authors"`
	Sections         []SectionDTO    `json:"sections"`
	References       []ReferenceDTO  `json:"references"`
	Figures          []FigureDTO     `json:"figures"`
	Card             json.RawMessage `json:"card,omitempty"`
}

type AuthorDTO struct {
	Order       int32   `json:"order"`
	DisplayName string  `json:"display_name"`
	ORCID       *string `json:"orcid,omitempty"`
}

type SectionDTO struct {
	Order     int32   `json:"order"`
	Heading   *string `json:"heading,omitempty"`
	Text      string  `json:"text"`
	PageStart *int32  `json:"page_start,omitempty"`
	PageEnd   *int32  `json:"page_end,omitempty"`
}

type ReferenceDTO struct {
	Order   int32    `json:"order"`
	Title   *string  `json:"title,omitempty"`
	Authors []string `json:"authors"`
	Venue   *string  `json:"venue,omitempty"`
	Year    *int32   `json:"year,omitempty"`
	DOI     *string  `json:"doi,omitempty"`
	RawText *string  `json:"raw_text,omitempty"`
}

type FigureDTO struct {
	Label   string `json:"label"`
	Kind    string `json:"kind"`
	Caption string `json:"caption"`
	Order   int32  `json:"order"`
	Page    *int32 `json:"page,omitempty"`
}

type PaperSummary struct {
	PaperID          uuid.UUID  `json:"paper_id"`
	Title            *string    `json:"title,omitempty"`
	Status           string     `json:"status"`
	PublicationYear  *int32     `json:"publication_year,omitempty"`
	UploadedFilename string     `json:"uploaded_filename"`
	CreatedAt        *time.Time `json:"created_at,omitempty"`
}

type SQLReadRepository struct {
	queries *db.Queries
}

func NewSQLReadRepository(queries *db.Queries) *SQLReadRepository {
	return &SQLReadRepository{queries: queries}
}

func (r *SQLReadRepository) GetJob(ctx context.Context, jobID uuid.UUID) (JobStatus, error) {
	job, err := r.queries.GetProcessingJob(ctx, jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return JobStatus{}, ErrNotFound
		}
		return JobStatus{}, err
	}
	return JobStatus{
		JobID:        job.ID,
		PaperID:      job.PaperID,
		Status:       job.Status,
		AttemptCount: job.AttemptCount,
		ErrorMessage: job.ErrorMessage,
		CreatedAt:    timestamp(job.CreatedAt),
		UpdatedAt:    timestamp(job.UpdatedAt),
		CompletedAt:  timestamp(job.CompletedAt),
	}, nil
}

func (r *SQLReadRepository) ListPapers(ctx context.Context) ([]PaperSummary, error) {
	rows, err := r.queries.ListPapers(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]PaperSummary, 0, len(rows))
	for _, row := range rows {
		summaries = append(summaries, PaperSummary{
			PaperID:          row.ID,
			Title:            row.Title,
			Status:           row.Status,
			PublicationYear:  row.PublicationYear,
			UploadedFilename: row.UploadedFilename,
			CreatedAt:        timestamp(row.CreatedAt),
		})
	}
	return summaries, nil
}

func (r *SQLReadRepository) GetPaperDetail(ctx context.Context, paperID uuid.UUID) (PaperDetail, error) {
	paper, err := r.queries.GetPaper(ctx, paperID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PaperDetail{}, ErrNotFound
		}
		return PaperDetail{}, err
	}
	authors, err := r.queries.ListPaperAuthors(ctx, paperID)
	if err != nil {
		return PaperDetail{}, err
	}
	sections, err := r.queries.ListPaperSections(ctx, paperID)
	if err != nil {
		return PaperDetail{}, err
	}
	references, err := r.queries.ListPaperReferences(ctx, paperID)
	if err != nil {
		return PaperDetail{}, err
	}

	detail := PaperDetail{
		PaperID:          paper.ID,
		SourceType:       paper.SourceType,
		Status:           paper.Status,
		Title:            paper.Title,
		Abstract:         paper.Abstract,
		DOI:              paper.Doi,
		PublicationYear:  paper.PublicationYear,
		UploadedFilename: paper.UploadedFilename,
		CreatedAt:        timestamp(paper.CreatedAt),
		UpdatedAt:        timestamp(paper.UpdatedAt),
		Authors:          make([]AuthorDTO, 0, len(authors)),
		Sections:         make([]SectionDTO, 0, len(sections)),
		References:       make([]ReferenceDTO, 0, len(references)),
	}
	for _, a := range authors {
		detail.Authors = append(detail.Authors, AuthorDTO{
			Order:       a.AuthorOrder,
			DisplayName: a.DisplayName,
			ORCID:       a.Orcid,
		})
	}
	for _, s := range sections {
		detail.Sections = append(detail.Sections, SectionDTO{
			Order:     s.SectionOrder,
			Heading:   s.Heading,
			Text:      s.Text,
			PageStart: s.PageStart,
			PageEnd:   s.PageEnd,
		})
	}
	for _, ref := range references {
		var names []string
		if len(ref.Authors) > 0 {
			_ = json.Unmarshal(ref.Authors, &names)
		}
		detail.References = append(detail.References, ReferenceDTO{
			Order:   ref.ReferenceOrder,
			Title:   ref.Title,
			Authors: names,
			Venue:   ref.Venue,
			Year:    ref.Year,
			DOI:     ref.Doi,
			RawText: ref.RawText,
		})
	}
	figures, err := r.queries.ListPaperFigures(ctx, paperID)
	if err != nil {
		return PaperDetail{}, err
	}
	detail.Figures = make([]FigureDTO, 0, len(figures))
	for _, f := range figures {
		detail.Figures = append(detail.Figures, FigureDTO{
			Label:   f.Label,
			Kind:    f.Kind,
			Caption: f.Caption,
			Order:   f.FigureOrder,
			Page:    f.Page,
		})
	}
	card, err := r.queries.GetLatestPaperCard(ctx, paperID)
	if err == nil {
		detail.Card = json.RawMessage(card.ContentJson)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return PaperDetail{}, err
	}
	return detail, nil
}

func (r *SQLReadRepository) CountPaperSections(ctx context.Context, paperID uuid.UUID) (int64, error) {
	return r.queries.CountPaperSections(ctx, paperID)
}

func (r *SQLReadRepository) ResetFailedJob(ctx context.Context, jobID uuid.UUID) (int64, error) {
	return r.queries.ResetFailedJob(ctx, jobID)
}

func (r *SQLReadRepository) SetJobTaskID(ctx context.Context, jobID uuid.UUID, taskID string) error {
	_, err := r.queries.SetProcessingJobTaskID(ctx, db.SetProcessingJobTaskIDParams{
		ID:     jobID,
		TaskID: &taskID,
	})
	return err
}

func timestamp(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}
