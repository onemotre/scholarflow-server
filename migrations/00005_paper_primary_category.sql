-- +goose Up
ALTER TABLE papers ADD COLUMN primary_category TEXT;

-- +goose Down
ALTER TABLE papers DROP COLUMN primary_category;
