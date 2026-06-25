# Figure & Diagram Image Extraction (P1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Crop every GROBID-detected figure out of the source PDF into a PNG, store it in MinIO, link it on `paper_figures.image_asset_id`, and serve it via a new API endpoint.

**Architecture:** A new best-effort, non-fatal stage in the worker pipeline runs after GROBID parse: it surfaces each figure's bounding box from the TEI `@coords`, shells out to poppler (`pdftoppm`) to render the figure's page, crops the bbox region, stores the PNG, and updates the figure row. The API server gains a single endpoint that streams a figure's image from MinIO. No DB migration (the `image_asset_id` column already exists) and no `scholarflow-web` change.

**Tech Stack:** Go (module `scholarflow_server`), chi router, pgx/v5 + sqlc, MinIO, asynq worker, poppler-utils (`pdftoppm`), Go stdlib `image`/`image/png`.

## Global Constraints

- Go module path is `scholarflow_server`; format every changed file with `go fmt ./...`.
- DB access is sqlc-generated: edit `queries/papers.sql`, then run `sqlc generate` (config `sqlc.yaml`, pgx/v5, UUIDs → `google/uuid`). Never hand-edit `internal/db/*.sql.go`.
- No schema migration — `paper_figures.image_asset_id` already exists (`migrations/00002_paper_figures.sql`).
- The figure-extraction stage is **best-effort and non-fatal**: any per-figure failure logs and continues; a stage-level failure never changes the job's status. Jobs still reach `parsed` (and `completed` if a reader is configured).
- GROBID `@coords` are `page,x,y,w,h` in PDF points, top-left origin (`px = point × dpi / 72`).
- All configuration is env vars with defaults, following `internal/config/config.go` (`envString`/`envBool`/`envInt64`).
- Tests use the standard `testing` package, co-located as `*_test.go`, and must not require Docker or network. The only test that needs the `pdftoppm` binary is build-tagged and skips when the binary is absent.
- Deviation from the design spec recorded here: figures are persisted inside `SaveParsedPaper` with a null `image_asset_id`, so the extraction stage **updates** the row via a new `SetPaperFigureImageAsset` query rather than setting `image_asset_id` at insert time. This keeps extraction a fully separate, non-fatal stage.

---

### Task 1: Parser surfaces the figure bounding box

**Files:**
- Modify: `internal/parser/parser.go` (add `FigureBox` type + `BBox` field on `Figure`)
- Modify: `internal/parser/grobid.go` (add `parseBox`, set `BBox` in the figure loop)
- Test: `internal/parser/grobid_test.go` (add `parseBox` table test)

**Interfaces:**
- Produces: `parser.FigureBox{Page int32; X, Y, W, H float64}` and `parser.Figure.BBox *FigureBox` (nil when no coords). Consumed by the pipeline crop stage in Task 4.

- [ ] **Step 1: Write the failing test**

Add to `internal/parser/grobid_test.go`:

```go
func TestParseBox(t *testing.T) {
	tests := []struct {
		name   string
		coords string
		want   *FigureBox
	}{
		{"empty", "", nil},
		{"malformed", "abc", nil},
		{"single", "3,10,20,30,40", &FigureBox{Page: 3, X: 10, Y: 20, W: 30, H: 40}},
		{"union same page", "3,10,20,5,5;3,30,40,5,5", &FigureBox{Page: 3, X: 10, Y: 20, W: 25, H: 25}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseBox(tt.coords)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("parseBox(%q) = %#v, want nil", tt.coords, got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Fatalf("parseBox(%q) = %#v, want %#v", tt.coords, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/parser/ -run TestParseBox -v`
Expected: FAIL — `undefined: FigureBox` / `undefined: parseBox`.

- [ ] **Step 3: Add the type and field**

In `internal/parser/parser.go`, add the type above `Figure` and a field to `Figure`:

```go
type FigureBox struct {
	Page       int32
	X, Y, W, H float64
}

type Figure struct {
	Order   int32
	Kind    string
	Label   string
	Caption string
	Page    *int32
	BBox    *FigureBox // nil when GROBID emitted no usable coords
}
```

- [ ] **Step 4: Implement `parseBox` and set it on each figure**

In `internal/parser/grobid.go`, add this function (next to `parsePage`):

```go
// parseBox unions the ";"-separated boxes in a GROBID @coords value into one
// rect on the dominant page (the page carrying the largest-area box). Returns
// nil when no box parses.
func parseBox(coords string) *FigureBox {
	coords = strings.TrimSpace(coords)
	if coords == "" {
		return nil
	}
	type box struct {
		page                   int32
		x0, y0, x1, y1, area   float64
	}
	var boxes []box
	for _, raw := range strings.Split(coords, ";") {
		parts := strings.Split(strings.TrimSpace(raw), ",")
		if len(parts) < 5 {
			continue
		}
		page, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || page <= 0 {
			continue
		}
		x, errX := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		y, errY := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		wv, errW := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
		hv, errH := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)
		if errX != nil || errY != nil || errW != nil || errH != nil {
			continue
		}
		boxes = append(boxes, box{page: int32(page), x0: x, y0: y, x1: x + wv, y1: y + hv, area: wv * hv})
	}
	if len(boxes) == 0 {
		return nil
	}
	dom := boxes[0]
	for _, b := range boxes[1:] {
		if b.area > dom.area {
			dom = b
		}
	}
	minX, minY, maxX, maxY := dom.x0, dom.y0, dom.x1, dom.y1
	for _, b := range boxes {
		if b.page != dom.page {
			continue
		}
		if b.x0 < minX {
			minX = b.x0
		}
		if b.y0 < minY {
			minY = b.y0
		}
		if b.x1 > maxX {
			maxX = b.x1
		}
		if b.y1 > maxY {
			maxY = b.y1
		}
	}
	return &FigureBox{Page: dom.page, X: minX, Y: minY, W: maxX - minX, H: maxY - minY}
}
```

In the same file, in the figure loop inside `parseTEI` (currently the `parsed.Figures = append(...)` block), add the `BBox` field:

```go
		parsed.Figures = append(parsed.Figures, Figure{
			Order:   int32(i + 1),
			Kind:    kind,
			Label:   label,
			Caption: strings.TrimSpace(fig.Desc),
			Page:    parsePage(fig.Coords),
			BBox:    parseBox(fig.Coords),
		})
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/parser/ -v`
Expected: PASS (including the existing `TestParseTEIExtractsFigures`).

- [ ] **Step 6: Commit**

```bash
go fmt ./internal/parser/
git add internal/parser/parser.go internal/parser/grobid.go internal/parser/grobid_test.go
git commit -m "feat(parser): surface figure bounding box from TEI coords"
```

---

### Task 2: Figure crop math + Cropper interface

**Files:**
- Create: `internal/figures/figures.go`
- Test: `internal/figures/figures_test.go`

**Interfaces:**
- Produces:
  - `figures.Rect{X, Y, W, H float64}` — a region in PDF points.
  - `figures.Cropper` interface: `Crop(ctx context.Context, pdfPath string, page int, rect Rect, dpi int) ([]byte, error)` (implemented in Task 3, consumed in Task 4).
  - `figures.PixelRect(rect Rect, dpi int, paddingPct float64, pageW, pageH int) image.Rectangle`.
  - `figures.Downscale(src image.Image, maxDim int) image.Image`.

- [ ] **Step 1: Write the failing test**

Create `internal/figures/figures_test.go`:

```go
package figures

import (
	"image"
	"testing"
)

func TestPixelRectScalesAndClamps(t *testing.T) {
	// 72->144 dpi doubles points->pixels; no padding; page big enough to not clamp.
	got := PixelRect(Rect{X: 10, Y: 20, W: 30, H: 40}, 144, 0, 1000, 1000)
	want := image.Rect(20, 40, 80, 120)
	if got != want {
		t.Fatalf("PixelRect = %v, want %v", got, want)
	}
}

func TestPixelRectClampsToPage(t *testing.T) {
	got := PixelRect(Rect{X: 0, Y: 0, W: 1000, H: 1000}, 72, 0, 50, 60)
	want := image.Rect(0, 0, 50, 60)
	if got != want {
		t.Fatalf("PixelRect = %v, want %v", got, want)
	}
}

func TestDownscaleLeavesSmallImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 50))
	got := Downscale(src, 2000)
	if got.Bounds().Dx() != 100 || got.Bounds().Dy() != 50 {
		t.Fatalf("Downscale changed small image: %v", got.Bounds())
	}
}

func TestDownscaleShrinksLargeImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4000, 2000))
	got := Downscale(src, 2000)
	if got.Bounds().Dx() != 2000 || got.Bounds().Dy() != 1000 {
		t.Fatalf("Downscale = %v, want 2000x1000", got.Bounds())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/figures/ -v`
Expected: FAIL — package/functions undefined.

- [ ] **Step 3: Implement the package**

Create `internal/figures/figures.go`:

```go
package figures

import (
	"context"
	"image"
)

// Rect is a figure region in PDF user-space points, top-left origin.
type Rect struct {
	X, Y, W, H float64
}

// Cropper renders the region rect on the 1-based page of the PDF at pdfPath and
// returns PNG bytes.
type Cropper interface {
	Crop(ctx context.Context, pdfPath string, page int, rect Rect, dpi int) ([]byte, error)
}

// PixelRect converts a points rect to a pixel rectangle at dpi, expands it by
// paddingPct on each side, and clamps it to the rendered page bounds.
func PixelRect(rect Rect, dpi int, paddingPct float64, pageW, pageH int) image.Rectangle {
	scale := float64(dpi) / 72.0
	x0 := rect.X * scale
	y0 := rect.Y * scale
	x1 := (rect.X + rect.W) * scale
	y1 := (rect.Y + rect.H) * scale
	padX := (x1 - x0) * paddingPct / 100.0
	padY := (y1 - y0) * paddingPct / 100.0
	r := image.Rect(int(x0-padX), int(y0-padY), int(x1+padX), int(y1+padY))
	return r.Intersect(image.Rect(0, 0, pageW, pageH))
}

// Downscale shrinks src with nearest-neighbor sampling so its longest side is at
// most maxDim pixels. It returns src unchanged when maxDim <= 0 or it already
// fits.
func Downscale(src image.Image, maxDim int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	longest := w
	if h > longest {
		longest = h
	}
	if maxDim <= 0 || longest <= maxDim {
		return src
	}
	ratio := float64(maxDim) / float64(longest)
	nw, nh := int(float64(w)*ratio), int(float64(h)*ratio)
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := b.Min.Y + int(float64(y)/ratio)
		for x := 0; x < nw; x++ {
			sx := b.Min.X + int(float64(x)/ratio)
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/figures/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
go fmt ./internal/figures/
git add internal/figures/figures.go internal/figures/figures_test.go
git commit -m "feat(figures): add crop math and Cropper interface"
```

---

### Task 3: Poppler cropper implementation

**Files:**
- Create: `internal/figures/poppler.go`
- Test: `internal/figures/poppler_test.go` (build-tagged integration test)
- Create: `internal/figures/testdata/sample.pdf` (a tiny one-page PDF fixture)

**Interfaces:**
- Consumes: `Rect`, `PixelRect`, `Downscale` from Task 2.
- Produces: `figures.NewPopplerCropper(paddingPct float64, maxDim int, workDir string) *PopplerCropper` implementing `Cropper`. Consumed by the worker wiring in Task 4.

- [ ] **Step 1: Implement the cropper**

Create `internal/figures/poppler.go`:

```go
package figures

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

// PopplerCropper renders a PDF page with pdftoppm and crops a figure region.
type PopplerCropper struct {
	binary  string
	padding float64
	maxDim  int
	workDir string
}

func NewPopplerCropper(paddingPct float64, maxDim int, workDir string) *PopplerCropper {
	return &PopplerCropper{binary: "pdftoppm", padding: paddingPct, maxDim: maxDim, workDir: workDir}
}

func (c *PopplerCropper) Crop(ctx context.Context, pdfPath string, page int, rect Rect, dpi int) ([]byte, error) {
	prefix := filepath.Join(c.workDir, fmt.Sprintf("sf-fig-%d-%d", page, os.Getpid()))
	// -singlefile makes pdftoppm write exactly <prefix>.png (no page-number suffix).
	cmd := execCommand(ctx, c.binary,
		"-png", "-singlefile",
		"-r", fmt.Sprintf("%d", dpi),
		"-f", fmt.Sprintf("%d", page),
		"-l", fmt.Sprintf("%d", page),
		pdfPath, prefix,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftoppm: %w: %s", err, out)
	}
	pngPath := prefix + ".png"
	defer os.Remove(pngPath)

	data, err := os.ReadFile(pngPath)
	if err != nil {
		return nil, fmt.Errorf("read rendered page: %w", err)
	}
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode rendered page: %w", err)
	}
	b := src.Bounds()
	crop := PixelRect(rect, dpi, c.padding, b.Dx(), b.Dy())
	if crop.Empty() {
		return nil, fmt.Errorf("empty crop rect for page %d", page)
	}
	sub := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	draw.Draw(sub, sub.Bounds(), src, crop.Min.Add(b.Min), draw.Src)

	var buf bytes.Buffer
	if err := png.Encode(&buf, Downscale(sub, c.maxDim)); err != nil {
		return nil, fmt.Errorf("encode crop: %w", err)
	}
	return buf.Bytes(), nil
}
```

Add a tiny indirection so the command is constructed in one place (also create `internal/figures/exec.go`):

```go
package figures

import (
	"context"
	"os/exec"
)

func execCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}
```

- [ ] **Step 2: Create the PDF fixture**

Generate a 1-page PDF fixture (requires poppler/cairo or any PDF producer available locally):

```bash
mkdir -p internal/figures/testdata
printf 'Figure extraction test page' | tee /tmp/sf-fixture.txt >/dev/null
# Produce a one-page PDF however is convenient locally, e.g. with libreoffice,
# pandoc, or: python3 -c "from fpdf import FPDF; p=FPDF(); p.add_page(); \
#   p.set_font('helvetica',size=24); p.cell(40,10,'fig'); p.output('internal/figures/testdata/sample.pdf')"
ls -l internal/figures/testdata/sample.pdf
```

Expected: `internal/figures/testdata/sample.pdf` exists (a valid one-page PDF).

- [ ] **Step 3: Write the build-tagged integration test**

Create `internal/figures/poppler_test.go`:

```go
//go:build integration

package figures

import (
	"context"
	"image/png"
	"bytes"
	"os/exec"
	"testing"
)

func TestPopplerCropperRendersRegion(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}
	c := NewPopplerCropper(2, 2000, t.TempDir())
	out, err := c.Crop(context.Background(), "testdata/sample.pdf", 1, Rect{X: 0, Y: 0, W: 100, H: 100}, 150)
	if err != nil {
		t.Fatalf("Crop returned error: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("output is not a valid PNG: %v", err)
	}
	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		t.Fatalf("empty crop image: %v", img.Bounds())
	}
}
```

- [ ] **Step 4: Verify the package builds and the gated test runs when poppler is present**

Run: `go build ./internal/figures/`
Expected: builds with no error.

Run (only meaningful where `pdftoppm` is installed): `go test -tags integration ./internal/figures/ -run TestPopplerCropper -v`
Expected: PASS, or SKIP when `pdftoppm` is absent. The default `go test ./internal/figures/` (no tag) must still PASS using only Task 2's tests.

- [ ] **Step 5: Commit**

```bash
go fmt ./internal/figures/
git add internal/figures/poppler.go internal/figures/exec.go internal/figures/poppler_test.go internal/figures/testdata/sample.pdf
git commit -m "feat(figures): add poppler-backed cropper"
```

---

### Task 4: Pipeline figure-extraction stage

**Files:**
- Modify: `queries/papers.sql` (add `SetPaperFigureImageAsset`)
- Regenerate: `internal/db/*.sql.go` via `sqlc generate`
- Modify: `internal/jobs/repository.go` (add `AttachFigureImage`)
- Modify: `internal/jobs/pipeline.go` (interface method, constructor param, `extractFigures` stage)
- Modify: `internal/config/config.go` (figure-extract config)
- Modify: `cmd/worker/main.go` (build cropper, pass to pipeline)
- Test: `internal/jobs/pipeline_test.go` (fake cropper + `AttachFigureImage` on fake repo, two new tests; update existing `NewPipeline` call sites)

**Interfaces:**
- Consumes: `parser.Figure.BBox` (Task 1), `figures.Cropper`/`figures.Rect` (Task 2/3).
- Produces:
  - `PipelineRepository.AttachFigureImage(ctx context.Context, paperID uuid.UUID, figureOrder int32, asset storage.Object) error`.
  - New `NewPipeline(repo PipelineRepository, store storage.Store, parser parser.Parser, readEnqueuer ReadEnqueuer, cropper figures.Cropper, figDPI int) *Pipeline` (cropper may be nil to disable extraction).
  - sqlc method `SetPaperFigureImageAsset(ctx, db.SetPaperFigureImageAssetParams{PaperID uuid.UUID, FigureOrder int32, ImageAssetID pgtype.UUID})`.

- [ ] **Step 1: Add the SQL query and regenerate**

Append to `queries/papers.sql`:

```sql
-- name: SetPaperFigureImageAsset :exec
UPDATE paper_figures
SET image_asset_id = sqlc.arg(image_asset_id)
WHERE paper_id = sqlc.arg(paper_id) AND figure_order = sqlc.arg(figure_order);
```

Run: `sqlc generate`
Expected: `internal/db/papers.sql.go` now contains `SetPaperFigureImageAsset` and `SetPaperFigureImageAssetParams`.

- [ ] **Step 2: Write the failing pipeline tests**

In `internal/jobs/pipeline_test.go`, first extend the existing fakes. Add a `figures` import and these fields/methods, and a fake cropper:

```go
// add import: "scholarflow_server/internal/figures"

type figureAttach struct {
	order int32
	asset storage.Object
}

// add to fakePipelineRepo struct:
//   attached  []figureAttach
//   failAttach error

func (r *fakePipelineRepo) AttachFigureImage(ctx context.Context, paperID uuid.UUID, figureOrder int32, asset storage.Object) error {
	if r.failAttach != nil {
		return r.failAttach
	}
	r.attached = append(r.attached, figureAttach{order: figureOrder, asset: asset})
	return nil
}

type fakeCropper struct {
	calls int
	err   error
}

func (c *fakeCropper) Crop(ctx context.Context, pdfPath string, page int, rect figures.Rect, dpi int) ([]byte, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	return []byte("PNGDATA"), nil
}
```

Also extend `fakePipelineStore` to record every Put (keep existing fields):

```go
// add to fakePipelineStore struct:
//   puts []struct{ Key, Body string }

// inside Put, after setting putKey/putBody:
//   s.puts = append(s.puts, struct{ Key, Body string }{key, string(data)})
```

Now add the two tests:

```go
func TestPipelineExtractsFigures(t *testing.T) {
	paperID := uuid.New()
	box := parser.FigureBox{Page: 2, X: 10, Y: 20, W: 30, H: 40}
	repo := &fakePipelineRepo{pdfAsset: storage.Object{Key: "papers/input.pdf"}}
	store := &fakePipelineStore{pdf: "pdfdata"}
	parserFake := &fakeParser{parsed: parser.ParsedPaper{
		Title:  "T",
		RawTEI: "<TEI/>",
		Figures: []parser.Figure{
			{Order: 1, Kind: "figure", Label: "Figure 1", BBox: &box},
			{Order: 2, Kind: "figure", Label: "Figure 2"}, // no BBox -> skipped
		},
	}}
	cropper := &fakeCropper{}
	service := NewPipeline(repo, store, parserFake, nil, cropper, 150)

	if err := service.ProcessPaper(context.Background(), ProcessPaperPayload{PaperID: paperID, JobID: uuid.New()}); err != nil {
		t.Fatalf("ProcessPaper error: %v", err)
	}
	if cropper.calls != 1 {
		t.Fatalf("cropper calls = %d, want 1", cropper.calls)
	}
	if len(repo.attached) != 1 || repo.attached[0].order != 1 {
		t.Fatalf("attached = %#v, want one entry for order 1", repo.attached)
	}
	wantKey := "papers/" + paperID.String() + "/figures/1.png"
	found := false
	for _, p := range store.puts {
		if p.Key == wantKey && p.Body == "PNGDATA" {
			found = true
		}
	}
	if !found {
		t.Fatalf("figure PNG not stored at %q; puts=%#v", wantKey, store.puts)
	}
	if got := strings.Join(repo.statuses, ","); got != "processing,parsed" {
		t.Fatalf("statuses = %s", got)
	}
}

func TestPipelineFigureExtractIsBestEffort(t *testing.T) {
	box := parser.FigureBox{Page: 1, X: 0, Y: 0, W: 10, H: 10}
	repo := &fakePipelineRepo{pdfAsset: storage.Object{Key: "papers/input.pdf"}}
	store := &fakePipelineStore{pdf: "pdfdata"}
	parserFake := &fakeParser{parsed: parser.ParsedPaper{
		Title:   "T",
		RawTEI:  "<TEI/>",
		Figures: []parser.Figure{{Order: 1, BBox: &box}},
	}}
	cropper := &fakeCropper{err: errors.New("pdftoppm boom")}
	service := NewPipeline(repo, store, parserFake, nil, cropper, 150)

	if err := service.ProcessPaper(context.Background(), ProcessPaperPayload{PaperID: uuid.New(), JobID: uuid.New()}); err != nil {
		t.Fatalf("ProcessPaper should not fail on crop error: %v", err)
	}
	if len(repo.attached) != 0 {
		t.Fatalf("attached = %#v, want none", repo.attached)
	}
	if got := strings.Join(repo.statuses, ","); got != "processing,parsed" {
		t.Fatalf("statuses = %s", got)
	}
}
```

Finally, update the three existing `NewPipeline(...)` call sites in this file to the new signature by appending `, nil, 0`:
- `TestPipelineProcessesPaperToParsed`
- `TestPipelineMarksJobFailedWhenParserFails`
- `TestPipelineEnqueuesReadAfterParse`

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/jobs/ -run TestPipeline -v`
Expected: FAIL — `AttachFigureImage` not in interface / `NewPipeline` arg count mismatch.

- [ ] **Step 4: Implement the repository method**

In `internal/jobs/repository.go`, add (it already imports `db`, `pgtype`, `storage`, `uuid`, `fmt`):

```go
func (r *SQLRepository) AttachFigureImage(ctx context.Context, paperID uuid.UUID, figureOrder int32, asset storage.Object) error {
	created, err := r.queries.CreatePaperAsset(ctx, db.CreatePaperAssetParams{
		PaperID:       paperID,
		AssetType:     "figure-image",
		StorageBucket: asset.Bucket,
		StorageKey:    asset.Key,
		ContentType:   asset.ContentType,
		SizeBytes:     asset.SizeBytes,
		Checksum:      stringPointer(asset.Checksum),
	})
	if err != nil {
		return fmt.Errorf("create figure image asset: %w", err)
	}
	if err := r.queries.SetPaperFigureImageAsset(ctx, db.SetPaperFigureImageAssetParams{
		PaperID:      paperID,
		FigureOrder:  figureOrder,
		ImageAssetID: pgtype.UUID{Bytes: created.ID, Valid: true},
	}); err != nil {
		return fmt.Errorf("set figure image asset: %w", err)
	}
	return nil
}
```

- [ ] **Step 5: Implement the pipeline stage**

In `internal/jobs/pipeline.go`:

Update imports to add `bytes`, `io`, `log`, `os`, and `scholarflow_server/internal/figures` (keep `context`, `fmt`, `strings`, `uuid`, `parser`, `storage`).

Add `AttachFigureImage` to the `PipelineRepository` interface:

```go
type PipelineRepository interface {
	UpdateJobStatus(ctx context.Context, jobID uuid.UUID, status string, errorMessage *string, attemptIncrement int32) error
	GetPaperPDFAsset(ctx context.Context, paperID uuid.UUID) (storage.Object, error)
	CreateTEIAsset(ctx context.Context, paperID uuid.UUID, asset storage.Object) error
	SaveParsedPaper(ctx context.Context, paperID uuid.UUID, parsed parser.ParsedPaper) error
	AttachFigureImage(ctx context.Context, paperID uuid.UUID, figureOrder int32, asset storage.Object) error
}
```

Change the struct and constructor:

```go
type Pipeline struct {
	repo         PipelineRepository
	store        storage.Store
	parser       parser.Parser
	readEnqueuer ReadEnqueuer
	cropper      figures.Cropper
	figDPI       int
}

func NewPipeline(repo PipelineRepository, store storage.Store, parser parser.Parser, readEnqueuer ReadEnqueuer, cropper figures.Cropper, figDPI int) *Pipeline {
	return &Pipeline{repo: repo, store: store, parser: parser, readEnqueuer: readEnqueuer, cropper: cropper, figDPI: figDPI}
}
```

In `process`, call the new stage after `SaveParsedPaper` succeeds (replace the trailing `return nil`):

```go
	if err := p.repo.SaveParsedPaper(ctx, payload.PaperID, parsed); err != nil {
		return fmt.Errorf("save parsed paper: %w", err)
	}
	p.extractFigures(ctx, payload.PaperID, pdfAsset.Key, parsed.Figures)
	return nil
```

Add the stage (best-effort, never returns an error):

```go
// extractFigures crops each figure with a bounding box out of the PDF and links
// the resulting image. It is best-effort: every failure is logged and skipped so
// the parse job is never affected.
func (p *Pipeline) extractFigures(ctx context.Context, paperID uuid.UUID, pdfKey string, figs []parser.Figure) {
	if p.cropper == nil {
		return
	}
	hasBox := false
	for _, f := range figs {
		if f.BBox != nil {
			hasBox = true
			break
		}
	}
	if !hasBox {
		return
	}
	// The original PDF reader was consumed by ParsePDF, so re-fetch it and write
	// it to a temp file that pdftoppm can open.
	rc, err := p.store.Get(ctx, pdfKey)
	if err != nil {
		log.Printf("figure extract: get pdf paper=%s: %v", paperID, err)
		return
	}
	defer rc.Close()
	tmp, err := os.CreateTemp("", "scholarflow-*.pdf")
	if err != nil {
		log.Printf("figure extract: temp file paper=%s: %v", paperID, err)
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		log.Printf("figure extract: copy pdf paper=%s: %v", paperID, err)
		return
	}
	tmp.Close()

	for _, f := range figs {
		if f.BBox == nil {
			continue
		}
		img, err := p.cropper.Crop(ctx, tmp.Name(), int(f.BBox.Page),
			figures.Rect{X: f.BBox.X, Y: f.BBox.Y, W: f.BBox.W, H: f.BBox.H}, p.figDPI)
		if err != nil {
			log.Printf("figure extract: crop paper=%s order=%d: %v", paperID, f.Order, err)
			continue
		}
		key := fmt.Sprintf("papers/%s/figures/%d.png", paperID.String(), f.Order)
		obj, err := p.store.Put(ctx, key, bytes.NewReader(img), int64(len(img)), "image/png")
		if err != nil {
			log.Printf("figure extract: put paper=%s order=%d: %v", paperID, f.Order, err)
			continue
		}
		if err := p.repo.AttachFigureImage(ctx, paperID, f.Order, obj); err != nil {
			log.Printf("figure extract: attach paper=%s order=%d: %v", paperID, f.Order, err)
			continue
		}
	}
}
```

- [ ] **Step 6: Add config**

In `internal/config/config.go`, add fields to `Config`:

```go
	FigureExtractEnabled    bool
	FigureExtractDPI        int
	FigureExtractPaddingPct int
	FigureExtractMaxDim     int
```

And in `Load()`:

```go
		FigureExtractEnabled:    envBool("FIGURE_EXTRACT_ENABLED", true),
		FigureExtractDPI:        int(envInt64("FIGURE_EXTRACT_DPI", 150)),
		FigureExtractPaddingPct: int(envInt64("FIGURE_EXTRACT_PADDING_PCT", 2)),
		FigureExtractMaxDim:     int(envInt64("FIGURE_EXTRACT_MAX_DIM", 2000)),
```

- [ ] **Step 7: Wire the cropper in the worker**

In `cmd/worker/main.go`, add imports `os` and `scholarflow_server/internal/figures`, and replace the pipeline construction line:

```go
	var cropper figures.Cropper
	if cfg.FigureExtractEnabled {
		cropper = figures.NewPopplerCropper(float64(cfg.FigureExtractPaddingPct), cfg.FigureExtractMaxDim, os.TempDir())
	}
	pipeline := jobs.NewPipeline(repo, store, parser.NewGROBIDParser(cfg.GROBIDURL), readEnqueuer, cropper, cfg.FigureExtractDPI)
```

- [ ] **Step 8: Run tests and build to verify they pass**

Run: `go test ./internal/jobs/ ./internal/config/ -v`
Expected: PASS (new and existing tests).

Run: `go build ./...`
Expected: builds with no error.

- [ ] **Step 9: Commit**

```bash
go fmt ./...
git add queries/papers.sql internal/db internal/jobs/repository.go internal/jobs/pipeline.go internal/jobs/pipeline_test.go internal/config/config.go cmd/worker/main.go
git commit -m "feat(jobs): crop and store figure images after parse"
```

---

### Task 5: Figure-image API endpoint

**Files:**
- Modify: `queries/papers.sql` (add `GetFigureImageAsset`)
- Regenerate: `internal/db/*.sql.go` via `sqlc generate`
- Modify: `internal/papers/read.go` (add `GetFigureImageKey`; add `ID`/`HasImage` to `FigureDTO` + set them)
- Create: `internal/httpapi/figures.go` (handler)
- Modify: `internal/httpapi/router.go` (Dependencies field + route)
- Modify: `cmd/server/main.go` (construct + wire handler)
- Test: `internal/httpapi/figures_test.go`

**Interfaces:**
- Consumes: `papers.ErrNotFound`, `storage.Store.Get`.
- Produces:
  - `papers.SQLReadRepository.GetFigureImageKey(ctx context.Context, paperID, figureID uuid.UUID) (string, error)`.
  - `httpapi.FigureImageHandler` + `httpapi.NewFigureImageHandler(reader FigureImageReader, store ObjectStore)`.
  - Route `GET /v1/papers/{id}/figures/{figureId}/image`.
  - `FigureDTO` gains `ID uuid.UUID json:"id"` and `HasImage bool json:"has_image"`.

- [ ] **Step 1: Add the SQL query and regenerate**

Append to `queries/papers.sql`:

```sql
-- name: GetFigureImageAsset :one
SELECT a.*
FROM paper_figures f
JOIN paper_assets a ON a.id = f.image_asset_id
WHERE f.id = sqlc.arg(figure_id) AND f.paper_id = sqlc.arg(paper_id);
```

Run: `sqlc generate`
Expected: `internal/db/papers.sql.go` now contains `GetFigureImageAsset` returning `PaperAsset` and `GetFigureImageAssetParams{FigureID uuid.UUID, PaperID uuid.UUID}`.

- [ ] **Step 2: Write the failing handler test**

Create `internal/httpapi/figures_test.go`:

```go
package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"scholarflow_server/internal/papers"
)

type fakeFigureReader struct {
	key string
	err error
}

func (f *fakeFigureReader) GetFigureImageKey(ctx context.Context, paperID, figureID uuid.UUID) (string, error) {
	return f.key, f.err
}

type fakeObjectStore struct {
	body string
}

func (s *fakeObjectStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(s.body)), nil
}

func TestGetFigureImageReturnsPNG(t *testing.T) {
	h := NewFigureImageHandler(&fakeFigureReader{key: "papers/x/figures/1.png"}, &fakeObjectStore{body: "PNGDATA"})
	router := NewRouter(Dependencies{FigureImageHandler: h})

	url := "/v1/papers/" + uuid.New().String() + "/figures/" + uuid.New().String() + "/image"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type = %q, want image/png", ct)
	}
	if rec.Body.String() != "PNGDATA" {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestGetFigureImageNotFound(t *testing.T) {
	h := NewFigureImageHandler(&fakeFigureReader{err: papers.ErrNotFound}, &fakeObjectStore{})
	router := NewRouter(Dependencies{FigureImageHandler: h})

	url := "/v1/papers/" + uuid.New().String() + "/figures/" + uuid.New().String() + "/image"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetFigureImageInvalidID(t *testing.T) {
	h := NewFigureImageHandler(&fakeFigureReader{}, &fakeObjectStore{})
	router := NewRouter(Dependencies{FigureImageHandler: h})

	url := "/v1/papers/" + uuid.New().String() + "/figures/not-a-uuid/image"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/httpapi/ -run TestGetFigureImage -v`
Expected: FAIL — `NewFigureImageHandler` / `Dependencies.FigureImageHandler` undefined.

- [ ] **Step 4: Implement the handler**

Create `internal/httpapi/figures.go`:

```go
package httpapi

import (
	"context"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type FigureImageReader interface {
	GetFigureImageKey(ctx context.Context, paperID, figureID uuid.UUID) (string, error)
}

type ObjectStore interface {
	Get(ctx context.Context, key string) (io.ReadCloser, error)
}

type FigureImageHandler struct {
	reader FigureImageReader
	store  ObjectStore
}

func NewFigureImageHandler(reader FigureImageReader, store ObjectStore) *FigureImageHandler {
	return &FigureImageHandler{reader: reader, store: store}
}

func (h *FigureImageHandler) GetFigureImage(w http.ResponseWriter, r *http.Request) {
	paperID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "invalid paper id", http.StatusBadRequest)
		return
	}
	figureID, err := uuid.Parse(chi.URLParam(r, "figureId"))
	if err != nil {
		http.Error(w, "invalid figure id", http.StatusBadRequest)
		return
	}
	key, err := h.reader.GetFigureImageKey(r.Context(), paperID, figureID)
	if err != nil {
		writeReadError(w, err) // maps papers.ErrNotFound -> 404
		return
	}
	obj, err := h.store.Get(r.Context(), key)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer obj.Close()
	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, obj)
}
```

- [ ] **Step 5: Register the route**

In `internal/httpapi/router.go`, add the field to `Dependencies`:

```go
type Dependencies struct {
	UploadHandler      *UploadHandler
	ReadHandler        *ReadHandler
	RetryHandler       *RetryHandler
	FigureImageHandler *FigureImageHandler
}
```

And inside `NewRouter`, after the `ReadHandler` block:

```go
	if deps.FigureImageHandler != nil {
		r.Get("/v1/papers/{id}/figures/{figureId}/image", deps.FigureImageHandler.GetFigureImage)
	}
```

- [ ] **Step 6: Implement the read-repo method and DTO fields**

In `internal/papers/read.go`, extend `FigureDTO`:

```go
type FigureDTO struct {
	ID       uuid.UUID `json:"id"`
	Label    string    `json:"label"`
	Kind     string    `json:"kind"`
	Caption  string    `json:"caption"`
	Order    int32     `json:"order"`
	Page     *int32    `json:"page,omitempty"`
	HasImage bool      `json:"has_image"`
}
```

Set the new fields in the figure loop of `GetPaperDetail`:

```go
		detail.Figures = append(detail.Figures, FigureDTO{
			ID:       f.ID,
			Label:    f.Label,
			Kind:     f.Kind,
			Caption:  f.Caption,
			Order:    f.FigureOrder,
			Page:     f.Page,
			HasImage: f.ImageAssetID.Valid,
		})
```

Add the resolver method (the file already imports `errors`, `pgx`, `uuid`, `db`):

```go
func (r *SQLReadRepository) GetFigureImageKey(ctx context.Context, paperID, figureID uuid.UUID) (string, error) {
	asset, err := r.queries.GetFigureImageAsset(ctx, db.GetFigureImageAssetParams{
		FigureID: figureID,
		PaperID:  paperID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return asset.StorageKey, nil
}
```

> If `internal/papers/read.go` does not already import `scholarflow_server/internal/db`, add it. Confirm the import block compiles in Step 8.

- [ ] **Step 7: Wire the handler in the server**

In `cmd/server/main.go`, after `readHandler := httpapi.NewReadHandler(readRepo)`:

```go
	figureImageHandler := httpapi.NewFigureImageHandler(readRepo, store)
```

And add it to the `Dependencies` literal in the `ListenAndServe` call:

```go
	httpapi.NewRouter(httpapi.Dependencies{
		UploadHandler:      uploadHandler,
		ReadHandler:        readHandler,
		RetryHandler:       retryHandler,
		FigureImageHandler: figureImageHandler,
	})
```

- [ ] **Step 8: Run tests and build to verify they pass**

Run: `go test ./internal/httpapi/ ./internal/papers/ -v`
Expected: PASS.

Run: `go build ./...`
Expected: builds with no error.

- [ ] **Step 9: Commit**

```bash
go fmt ./...
git add queries/papers.sql internal/db internal/papers/read.go internal/httpapi/figures.go internal/httpapi/figures_test.go internal/httpapi/router.go cmd/server/main.go
git commit -m "feat(api): serve extracted figure images"
```

---

### Task 6: Deployment, config docs, and changelog

**Files:**
- Modify: `Dockerfile` (install `poppler-utils` in the runtime image)
- Modify: `.env.example` (document the figure-extract vars)
- Modify: `docs/api.md` (document the new endpoint + `has_image`/`id` fields)
- Modify: `README.md` (add an Acknowledgements section crediting dependencies, incl. poppler)
- Modify: `CHANGELOG.md` (version entry)

**Interfaces:** none (deployment + docs only).

- [ ] **Step 1: Add poppler to the runtime image**

In `Dockerfile`, change the runtime `apt-get install` line to include `poppler-utils`:

```dockerfile
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates poppler-utils \
    && rm -rf /var/lib/apt/lists/*
```

- [ ] **Step 2: Document the config vars**

Append to `.env.example`:

```dotenv
# Figure image extraction (worker). Crops GROBID-detected figures into PNGs.
FIGURE_EXTRACT_ENABLED=true
FIGURE_EXTRACT_DPI=150
FIGURE_EXTRACT_PADDING_PCT=2
FIGURE_EXTRACT_MAX_DIM=2000
```

- [ ] **Step 3: Document the endpoint**

In `docs/api.md`, add an entry describing:

```markdown
### GET /v1/papers/{id}/figures/{figureId}/image

Streams the extracted PNG for a figure. `figureId` is the `id` from a figure in
`GET /v1/papers/{id}`. Responses:

- `200 OK` with `Content-Type: image/png` — the cropped figure image.
- `404 Not Found` — the figure does not exist or has no extracted image.
- `400 Bad Request` — malformed `id` or `figureId`.

Each figure object in `GET /v1/papers/{id}` now includes `"id"` (UUID) and
`"has_image"` (bool) so clients know which figures have an image to fetch.
```

- [ ] **Step 4: Add the README Acknowledgements section**

The `scholarflow-server/README.md` has no acknowledgements section. Append one at
the end of the file (after the `## TODO ☑️` section) crediting the third-party
tools the pipeline relies on, including the new poppler dependency:

```markdown
## 致谢 / Acknowledgements 🙏

本项目依赖以下开源工具：

- [GROBID](https://github.com/kermitt2/grobid) — scholarly PDF 解析为 TEI XML
- [Poppler](https://poppler.freedesktop.org/) (`pdftoppm`) — 渲染 PDF 页面以裁剪图表/系统架构图
- [asynq](https://github.com/hibiken/asynq) — Redis 异步任务队列
- [sqlc](https://sqlc.dev/) — 从 SQL 生成类型安全的 Go 数据访问代码
- [goose](https://github.com/pressly/goose) — 数据库迁移
```

- [ ] **Step 5: Add the changelog entry**

Prepend a new version section to `CHANGELOG.md` (use the date `2026-06-24`):

```markdown
## [0.7.0] - 2026-06-24
### Features
- Figure & diagram image extraction: the worker crops each GROBID-detected figure
  out of the source PDF (poppler `pdftoppm` + bbox from TEI `@coords`), stores the
  PNG in MinIO, and links it on `paper_figures.image_asset_id`.
- New endpoint `GET /v1/papers/{id}/figures/{figureId}/image` streams a figure
  image; figures in `GET /v1/papers/{id}` now expose `id` and `has_image`.
### Design Rationale
- Extraction is a best-effort, non-fatal pipeline stage so a bad bounding box never
  fails parsing; jobs still reach `parsed`.
- Poppler shell-out was chosen over cgo/MuPDF (AGPL linking) and pdffigures2 (heavy
  JVM service) for the lowest added dependency; the `Cropper` interface keeps the
  pipeline unit-testable without the binary.
### Notes & Caveats
- The worker image now requires `poppler-utils`.
- GROBID bboxes can clip multi-panel figures or omit captions; padding mitigates.
  Architecture-diagram selection and the HTML fallback are deferred to later work.
```

- [ ] **Step 6: Verify the build still compiles**

Run: `go build ./...`
Expected: builds with no error (this task changes no Go code, but confirm nothing was left broken).

- [ ] **Step 7: Commit**

```bash
git add Dockerfile .env.example docs/api.md README.md CHANGELOG.md
git commit -m "docs: document figure extraction config, endpoint, acknowledgements, and changelog"
```

---

## Self-Review

**Spec coverage:**
- Parser surfaces bbox → Task 1. ✓
- Poppler crop engine + interface + math → Tasks 2, 3. ✓
- Best-effort non-fatal pipeline stage, deterministic keys, `image_asset_id` set → Task 4. ✓
- Config vars (enabled/dpi/padding/max_dim) → Task 4. ✓
- No migration; sqlc queries + regen → Tasks 4, 5. ✓
- Image-serving endpoint + `has_image` → Task 5. ✓
- Dockerfile poppler, docs, changelog → Task 6. ✓
- Non-goals (architecture-diagram selection, web rendering) → explicitly deferred. ✓

**Type consistency:** `figures.Cropper.Crop(ctx, pdfPath string, page int, rect Rect, dpi int)` is identical in the interface (Task 2), poppler impl (Task 3), fake (Task 4), and pipeline call (Task 4). `AttachFigureImage(ctx, paperID, figureOrder int32, asset storage.Object) error` matches across interface, fake, and impl. `GetFigureImageKey(ctx, paperID, figureID uuid.UUID) (string, error)` matches handler interface and repo impl. sqlc names (`SetPaperFigureImageAsset`, `GetFigureImageAsset`, `GetFigureImageAssetParams{FigureID, PaperID}`) are referenced consistently.

**Placeholder scan:** no TBD/TODO; every code step shows complete code. The only non-literal step is the PDF fixture in Task 3 (a binary asset), which gives concrete generation options and a verifiable existence check.
