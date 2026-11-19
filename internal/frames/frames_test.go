package frames

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// halves returns a w x h image whose left half is red and right half blue.
func halves(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{R: 255, A: 255}
			if x >= w/2 {
				c = color.RGBA{B: 255, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestCoverCropsWiderSource(t *testing.T) {
	// 400x100 -> 100x100 keeps the middle 100px: 50px of red, 50px of blue.
	got := Cover(halves(400, 100), 100, 100)
	if got.Bounds() != image.Rect(0, 0, 100, 100) {
		t.Fatalf("bounds = %v", got.Bounds())
	}
	if c := got.RGBAAt(10, 50); c.R < 200 || c.B > 50 {
		t.Errorf("left side should be red, got %v", c)
	}
	if c := got.RGBAAt(90, 50); c.B < 200 || c.R > 50 {
		t.Errorf("right side should be blue, got %v", c)
	}
	if len(got.Pix) != 100*100*4 || got.Stride != 400 {
		t.Errorf("frame must be tightly packed: stride %d", got.Stride)
	}
}

func TestCoverCropsTallerSource(t *testing.T) {
	got := Cover(halves(100, 400), 50, 100)
	if got.Bounds().Dx() != 50 || got.Bounds().Dy() != 100 {
		t.Fatalf("bounds = %v", got.Bounds())
	}
	if c := got.RGBAAt(5, 50); c.R < 200 {
		t.Errorf("left should be red, got %v", c)
	}
}

func TestToRGBANormalizesBounds(t *testing.T) {
	sub := halves(10, 10).SubImage(image.Rect(5, 5, 10, 10))
	got := ToRGBA(sub)
	if got.Bounds() != image.Rect(0, 0, 5, 5) {
		t.Errorf("bounds = %v", got.Bounds())
	}
	if c := got.RGBAAt(0, 0); c.B != 255 {
		t.Errorf("pixel = %v, want blue", c)
	}
}

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, halves(4, 2)); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	img, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if img.Bounds().Dx() != 4 {
		t.Errorf("bounds = %v", img.Bounds())
	}

	bad := filepath.Join(t.TempDir(), "bad.png")
	_ = os.WriteFile(bad, []byte("not an image"), 0o600)
	if _, err := Load(bad); err == nil {
		t.Error("expected decode error")
	}
}
