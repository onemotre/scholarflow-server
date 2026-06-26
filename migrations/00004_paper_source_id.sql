-- +goose Up
ALTER TABLE papers ADD COLUMN source_id TEXT;
CREATE UNIQUE INDEX papers_arxiv_source_id_key
    ON papers (source_id)
    WHERE source_type = 'arxiv' AND source_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS papers_arxiv_source_id_key;
ALTER TABLE papers DROP COLUMN source_id;
