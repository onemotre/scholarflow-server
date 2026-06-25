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
