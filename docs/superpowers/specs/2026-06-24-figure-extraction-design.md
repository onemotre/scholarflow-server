# Figure & Diagram Image Extraction (P1) — Design

**Date:** 2026-06-24
**Status:** Approved (brainstorming)
**Module:** `scholarflow-server`

## Context

This is the first of three sub-projects in a larger effort to improve paper-reading
quality. The full effort decomposes into:

- **P1 (this spec)** — Figure & diagram image extraction: crop GROBID-detected
  figures from the PDF into PNGs, store them, and serve them via the API.
- **P2** — Card schema v3 (Introduction / Methodology with problem↔method mapping /
  metric-centric Results / module-based Implementation). *Not in this spec.*
- **P3** — Multi-agent reader (Analyst → Explainer → Synthesizer) that fills schema
  v3 with grounded evidence and figure references. *Not in this spec.*

Sequencing is P1 → P2 → P3. P1 is the foundation because every "locate the figure"
requirement in P2/P3 depends on figure images existing.

### Why GROBID alone is not enough

The pipeline already requests `teiCoordinates=figure` from GROBID
(`internal/parser/grobid.go`), so each `<figure>` carries a bounding box
(`page,x,y,w,h`) plus label and caption. GROBID's output stops at TEI XML — it does
**not** rasterize/crop the region into an image. `paper_figures.image_asset_id`
already exists in the schema (`migrations/00002_paper_figures.sql`) but is unused
today: the schema was designed for an extracted image that nothing currently
produces. P1 produces it.

The crop engine decision (evaluated: poppler shell-out, go-fitz/MuPDF cgo, PyMuPDF
sidecar, pdffigures2 service) resolved to **poppler shell-out**: no cgo, no AGPL
linking (separate binary), robust for vector architecture diagrams, lowest added
infrastructure.

## Scope

**In scope:** server-side extraction only — parser bbox surfacing, poppler crop
stage in the worker pipeline, PNG storage + `image_asset_id` population, and a
single image-serving API endpoint.

**Out of scope (deferred):**

- Architecture-diagram *selection* and the HTML-diagram fallback — that is the
  reader's job (P2/P3); the reader decides which figure is the system diagram.
- Caption-region inclusion and multi-panel correction (GROBID bbox may clip;
  padding mitigates; pdffigures2 is a future upgrade path).
- Any `scholarflow-web` rendering — web wiring waits until the v3 card references
  figures (P2/P3).

## Components

### 1. Parser — surface the bounding box (`internal/parser`)

Extend `Figure` with an optional bounding box:

```go
type FigureBox struct {
    Page    int32
    X, Y, W, H float64
}

type Figure struct {
    Order   int32
    Kind    string
    Label   string
    Caption string
    Page    *int32
    BBox    *FigureBox // nil when GROBID emitted no coords
}
```

Parse `BBox` from `teiFigure.Coords` (today only the page is kept via `parsePage`;
the `x,y,w,h` is discarded). GROBID may emit several `;`-separated boxes — union
them into one rect on the dominant page (the page of the largest box). Figures with
no coords get `BBox = nil`. Pure function, table-driven tests.

### 2. Crop engine (new package `internal/figures`)

```go
type Cropper interface {
    Crop(ctx context.Context, pdfPath string, box FigureBox, dpi int) ([]byte, error)
}
```

Interface keeps the pipeline mockable (no poppler needed in unit tests).

Poppler implementation:

1. `pdftoppm -png -r <dpi> -f <page> -l <page> <pdf> <tmpPrefix>` renders the single
   page to a temp PNG in the scratch dir.
2. Convert GROBID points to pixels: `px = point × dpi / 72` (coords are top-left
   origin, matching raster image origin).
3. Apply padding (`PADDING_PCT`), clamp the rect to page raster bounds.
4. Crop with `image` / `image/png` stdlib, cap to max output dimension, re-encode PNG.

The points→pixels + padding + clamp computation is a separate pure function, tested
directly without poppler.

Config (env, defaults):

| Var | Default | Meaning |
|-----|---------|---------|
| `FIGURE_EXTRACT_ENABLED` | `true` | master switch for the stage |
| `FIGURE_EXTRACT_DPI` | `150` | render DPI |
| `FIGURE_EXTRACT_PADDING_PCT` | `2` | padding added around the bbox |
| `FIGURE_EXTRACT_MAX_DIM` | `2000` | max output pixel dimension (longest side) |

### 3. Pipeline stage (`internal/jobs/pipeline.go`)

After GROBID parse, write the already-fetched PDF bytes (`p.store.Get`) to one temp
file. For each figure with a `BBox`:

1. `Crop` the region.
2. `store.Put` the PNG at deterministic key `papers/{paperID}/figures/{order}.png`.
3. `CreatePaperAsset` with `asset_type = "figure-image"`.
4. `CreatePaperFigure` with `image_asset_id` set (the column already accepts it — no
   migration, no separate UPDATE).

**Best-effort per figure:** any crop/store error logs and continues, leaving
`image_asset_id` null for that figure. **The stage is non-fatal:** figures are
auxiliary, so a stage-level failure logs but never flips the job off its `parsed`
path. Reprocessing is idempotent via deterministic object keys.

### 4. DB / sqlc

No migration (`image_asset_id` already exists; `CreatePaperAsset` and
`CreatePaperFigure` already exist). Add one read query `GetPaperAssetByID` for the
serving endpoint, then regenerate `internal/db` with `sqlc generate`.

### 5. API (`internal/httpapi`)

New route:

```
GET /v1/papers/{id}/figures/{figureId}/image
```

Resolve figure → `image_asset_id` → asset `storage_key` → `store.Get` → stream with
`Content-Type: image/png`. Return 404 when the figure has no image.

Add a `has_image` boolean to each figure entry in the existing `GET /v1/papers/{id}`
response so clients know which figures have an extractable image.

## Data Flow

```
PDF (MinIO)
  → GROBID TEI (with @coords)
  → parser.Figure{BBox}
  → [poppler render page → crop bbox → PNG]
  → MinIO object + paper_assets row + paper_figures.image_asset_id
  → GET /v1/papers/{id}/figures/{figureId}/image streams on demand
```

## Error Handling

- No coords → `BBox = nil` → skip, `image_asset_id` stays null.
- poppler missing or render failure → log (one-time warning for missing binary),
  skip the figure, continue.
- Crop rect out of page bounds → clamp to raster bounds.
- The extraction stage never fails the parse job.

## Testing

- **Unit (no Docker):**
  - parser bbox parse + multi-box union (table-driven).
  - crop pixel-math: points→pixels, padding, clamping (pure function).
  - pipeline stage with a fake `Cropper`: asserts best-effort behavior, deterministic
    keys, asset + figure writes, and that a crop error leaves `image_asset_id` null
    without failing the job.
  - API handler with a fake store: body bytes, `image/png` content-type, 404 path.
- **Integration (optional, build-tagged):** a real-PDF fixture exercising the poppler
  `Cropper`; skips when the `pdftoppm` binary is absent so CI stays Docker/binary-free.

## Deployment Notes

- `pdftoppm` (poppler-utils) must be present in the worker container — Dockerfile
  change required.
- No schema migration. No `scholarflow-web` change.

## Known Caveats

- GROBID figure bboxes can exclude captions or clip multi-panel/composite figures;
  padding mitigates but does not fully solve this. pdffigures2 is the future upgrade
  path if boundary quality proves insufficient.
- Large figures are size-capped (`FIGURE_EXTRACT_MAX_DIM`) to bound object size.
