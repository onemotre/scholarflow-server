//go:build integration

package figures

import (
	"bytes"
	"context"
	"image/png"
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
