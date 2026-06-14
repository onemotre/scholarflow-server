-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE papers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source_type TEXT NOT NULL,
    status TEXT NOT NULL,
    title TEXT,
    abstract TEXT,
    doi TEXT,
    publication_year INTEGER,
    uploaded_filename TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE paper_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    asset_type TEXT NOT NULL,
    storage_bucket TEXT NOT NULL,
    storage_key TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    checksum TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE paper_processing_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    task_id TEXT,
    error_message TEXT,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE TABLE paper_authors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    author_order INTEGER NOT NULL,
    display_name TEXT NOT NULL,
    orcid TEXT,
    openalex_author_id TEXT,
    UNIQUE (paper_id, author_order)
);

CREATE TABLE institutions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name TEXT NOT NULL,
    ror_id TEXT,
    openalex_id TEXT,
    country_code TEXT,
    raw_names JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE paper_institutions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    institution_id UUID REFERENCES institutions(id) ON DELETE SET NULL,
    raw_affiliation TEXT NOT NULL,
    role TEXT NOT NULL,
    source TEXT NOT NULL,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE paper_sections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    section_order INTEGER NOT NULL,
    heading TEXT,
    text TEXT NOT NULL,
    page_start INTEGER,
    page_end INTEGER,
    grobid_path TEXT,
    UNIQUE (paper_id, section_order)
);

CREATE TABLE paper_references (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    reference_order INTEGER NOT NULL,
    title TEXT,
    authors JSONB NOT NULL DEFAULT '[]'::jsonb,
    venue TEXT,
    year INTEGER,
    doi TEXT,
    raw_text TEXT,
    UNIQUE (paper_id, reference_order)
);

CREATE TABLE paper_cards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    schema_version TEXT NOT NULL,
    model TEXT NOT NULL,
    content_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE paper_evidence (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    paper_id UUID NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    paper_card_id UUID REFERENCES paper_cards(id) ON DELETE CASCADE,
    claim_key TEXT NOT NULL,
    evidence_type TEXT NOT NULL,
    section_id UUID REFERENCES paper_sections(id) ON DELETE SET NULL,
    asset_id UUID REFERENCES paper_assets(id) ON DELETE SET NULL,
    page INTEGER,
    locator TEXT,
    snippet TEXT,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_paper_assets_paper_id ON paper_assets(paper_id);
CREATE INDEX idx_jobs_paper_id ON paper_processing_jobs(paper_id);
CREATE INDEX idx_jobs_status ON paper_processing_jobs(status);
CREATE INDEX idx_sections_paper_id ON paper_sections(paper_id);
CREATE INDEX idx_cards_paper_id ON paper_cards(paper_id);
CREATE INDEX idx_evidence_paper_id ON paper_evidence(paper_id);

-- +goose Down
DROP TABLE IF EXISTS paper_evidence;
DROP TABLE IF EXISTS paper_cards;
DROP TABLE IF EXISTS paper_references;
DROP TABLE IF EXISTS paper_sections;
DROP TABLE IF EXISTS paper_institutions;
DROP TABLE IF EXISTS institutions;
DROP TABLE IF EXISTS paper_authors;
DROP TABLE IF EXISTS paper_processing_jobs;
DROP TABLE IF EXISTS paper_assets;
DROP TABLE IF EXISTS papers;
