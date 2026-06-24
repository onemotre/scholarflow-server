-- name: CreatePaper :one
INSERT INTO papers (source_type, status, uploaded_filename)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdatePaperMetadata :one
UPDATE papers
SET title = $2,
    abstract = $3,
    doi = $4,
    publication_year = $5,
    status = $6,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: GetPaper :one
SELECT * FROM papers WHERE id = $1;

-- name: CreatePaperAsset :one
INSERT INTO paper_assets (paper_id, asset_type, storage_bucket, storage_key, content_type, size_bytes, checksum)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: CreateProcessingJob :one
INSERT INTO paper_processing_jobs (paper_id, status)
VALUES ($1, $2)
RETURNING *;

-- name: SetProcessingJobTaskID :one
UPDATE paper_processing_jobs
SET task_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: UpdateProcessingJobStatus :one
UPDATE paper_processing_jobs
SET status = $2,
    error_message = $3,
    attempt_count = attempt_count + $4,
    updated_at = now(),
    completed_at = CASE WHEN $2 IN ('completed', 'failed') THEN now() ELSE completed_at END
WHERE id = $1
RETURNING *;

-- name: GetProcessingJob :one
SELECT * FROM paper_processing_jobs WHERE id = $1;

-- name: GetPaperAssetByType :one
SELECT * FROM paper_assets
WHERE paper_id = $1 AND asset_type = $2
ORDER BY created_at DESC
LIMIT 1;

-- name: DeletePaperAuthors :exec
DELETE FROM paper_authors WHERE paper_id = $1;

-- name: CreatePaperAuthor :one
INSERT INTO paper_authors (paper_id, author_order, display_name, orcid)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: DeletePaperSections :exec
DELETE FROM paper_sections WHERE paper_id = $1;

-- name: CreatePaperSection :one
INSERT INTO paper_sections (paper_id, section_order, heading, text, page_start, page_end, grobid_path)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: DeletePaperReferences :exec
DELETE FROM paper_references WHERE paper_id = $1;

-- name: CreatePaperReference :one
INSERT INTO paper_references (paper_id, reference_order, title, authors, venue, year, doi, raw_text)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: ListPaperAuthors :many
SELECT * FROM paper_authors WHERE paper_id = $1 ORDER BY author_order;

-- name: ListPaperSections :many
SELECT * FROM paper_sections WHERE paper_id = $1 ORDER BY section_order;

-- name: ListPaperReferences :many
SELECT * FROM paper_references WHERE paper_id = $1 ORDER BY reference_order;

-- name: CreatePaperCard :one
INSERT INTO paper_cards (paper_id, schema_version, model, content_json)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: CreatePaperEvidence :one
INSERT INTO paper_evidence (paper_id, paper_card_id, claim_key, evidence_type, section_id, asset_id, page, locator, snippet, confidence)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: DeletePaperCardsByPaper :exec
DELETE FROM paper_cards WHERE paper_id = $1;

-- name: GetLatestPaperCard :one
SELECT * FROM paper_cards WHERE paper_id = $1 ORDER BY created_at DESC LIMIT 1;

-- name: DeletePaperFiguresByPaper :exec
DELETE FROM paper_figures WHERE paper_id = $1;

-- name: CreatePaperFigure :one
INSERT INTO paper_figures (paper_id, kind, label, caption, figure_order, page, image_asset_id)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: ListPaperFigures :many
SELECT * FROM paper_figures WHERE paper_id = $1 ORDER BY figure_order;

-- name: SetJobStatusAndAttempt :one
UPDATE paper_processing_jobs
SET status = $2,
    error_message = $3,
    attempt_count = $4,
    updated_at = now(),
    completed_at = CASE WHEN $2 IN ('completed', 'failed') THEN now() ELSE completed_at END
WHERE id = $1
RETURNING *;

-- name: ResetFailedJob :execrows
UPDATE paper_processing_jobs
SET status = 'queued',
    attempt_count = 0,
    error_message = NULL,
    completed_at = NULL,
    updated_at = now()
WHERE id = $1 AND status = 'failed';

-- name: CountPaperSections :one
SELECT count(*) FROM paper_sections WHERE paper_id = $1;

-- name: DeleteFailedJobsOlderThan :execrows
DELETE FROM paper_processing_jobs
WHERE status = 'failed' AND updated_at < $1;

-- name: ListPapers :many
SELECT id, title, status, publication_year, uploaded_filename, created_at
FROM papers
ORDER BY created_at DESC
LIMIT 500;
