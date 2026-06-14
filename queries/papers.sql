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
