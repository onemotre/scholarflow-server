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
