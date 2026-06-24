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
