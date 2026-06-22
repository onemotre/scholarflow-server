-- +goose Up
CREATE TABLE paper_figures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    label TEXT NOT NULL,
    caption TEXT NOT NULL,
    figure_order INTEGER NOT NULL,
    page INTEGER,
    image_asset_id UUID REFERENCES paper_assets(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_figures_paper_id ON paper_figures(paper_id);

-- +goose Down
DROP TABLE IF EXISTS paper_figures;
