-- +goose Up
ALTER TABLE paper_sections ADD COLUMN section_number TEXT;

-- +goose Down
ALTER TABLE paper_sections DROP COLUMN section_number;
