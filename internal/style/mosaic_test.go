package style

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"slices"
	"strings"
	"testing"

	"github.com/nothinglefttolearn/nltl-animate/internal/preset"
)

func TestMosaicSequenceRules(t *testing.T) {
	for _, c := range []struct{ images, tiles int }{
		{2, 1}, {2, 4}, {3, 4}, {4, 4}, {5, 4}, {9, 9}, {10, 9}, {4, 9}, {7, 16}, {30, 144}, {2, 144},
	} {
		for seed := range uint64(5) {
			steps := MosaicSequence(c.images, c.tiles, 60, seed)
			for s, step := range steps {
				if len(step) != c.tiles {
					t.Fatalf("%+v: step %d has %d tiles", c, s, len(step))
				}
				counts := make([]int, c.images)
				for i, img := range step {
					if s > 0 && img == steps[s-1][i] {
						t.Fatalf("%+v seed %d: tile %d kept image %d at step %d", c, seed, i, img, s)
					}
					counts[img]++
				}
				// Spread as evenly as possible; all different when there
				// are enough images.
				lo, hi := c.tiles/c.images, (c.tiles+c.images-1)/c.images
				for img, n := range counts {
					if c.tiles > 1 && (n < lo || n > hi) {
						t.Fatalf("%+v seed %d step %d: image %d used %d times, want %d–%d", c, seed, s, img, n, lo, hi)
					}
				}
			}
		}
	}
}

func TestMosaicSequenceTilesDifferAndIsDeterministic(t *testing.T) {
	a := MosaicSequence(6, 4, 30, 1)
	if !slices.EqualFunc(a, MosaicSequence(6, 4, 30, 1), slices.Equal) {
		t.Error("same seed should give the same sequence")
	}
	if slices.EqualFunc(a, MosaicSequence(6, 4, 30, 2), slices.Equal) {
		t.Error("different seeds should differ")
	}
	tile := func(i int) []int {
		var seq []int
		for _, step := range a {
			seq = append(seq, step[i])
		}
		return seq
	}
	for i := 1; i < 4; i++ {
		if slices.Equal(tile(0), tile(i)) {
			t.Errorf("tile %d has the same sequence as tile 0", i)
		}
	}
}

func TestMosaicWriteFrames(t *testing.T) {
	cols := []color.RGBA{{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255}}
	var imgs []*image.RGBA
	for _, c := range cols {
		imgs = append(imgs, solid(4, 2, c))
	}
	const n, hold, frames = 2, 3, 8
	m, err := NewMosaic(imgs, n, hold, 5)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := m.Size(); w != 8 || h != 4 {
		t.Fatalf("size %dx%d, want 8x4", w, h)
	}
	var buf bytes.Buffer
	if err := m.WriteFrames(&buf, frames); err != nil {
		t.Fatal(err)
	}
	frameBytes := 8 * 4 * 4
	if buf.Len() != frames*frameBytes {
		t.Fatalf("wrote %d bytes, want %d", buf.Len(), frames*frameBytes)
	}

	steps := MosaicSequence(3, 4, 3, 5)
	for f := range frames {
		img := &image.RGBA{Pix: buf.Bytes()[f*frameBytes : (f+1)*frameBytes], Stride: 8 * 4, Rect: image.Rect(0, 0, 8, 4)}
		for tile, want := range steps[f/hold] {
			// Check every pixel of the tile.
			for y := range 2 {
				for x := range 4 {
					px, py := (tile%n)*4+x, (tile/n)*2+y
					if got := img.RGBAAt(px, py); got != cols[want] {
						t.Fatalf("frame %d tile %d (%d,%d) = %v, want image %d", f, tile, px, py, got, want)
					}
				}
			}
		}
	}
}

func TestNewMosaicErrors(t *testing.T) {
	a, b := solid(4, 4, color.RGBA{A: 255}), solid(4, 4, color.RGBA{A: 255})
	for name, err := range map[string]error{
		"one image":  must(NewMosaic([]*image.RGBA{a}, 2, 1, 1)),
		"zero grid":  must(NewMosaic([]*image.RGBA{a, b}, 0, 1, 1)),
		"zero hold":  must(NewMosaic([]*image.RGBA{a, b}, 2, 0, 1)),
		"size mixup": must(NewMosaic([]*image.RGBA{a, solid(5, 4, color.RGBA{})}, 2, 1, 1)),
	} {
		if err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func must(_ *Mosaic, err error) error { return err }

func TestMosaicFilter(t *testing.T) {
	p, _ := preset.Get("industrial")
	f := MosaicFilter(p, 1920, 1080, 3, 120)
	tw, th := GridTileSize(1920, 1080, 3)
	if !strings.HasPrefix(f, "split=9[s0]") {
		t.Errorf("should start by splitting the unlabelled input: %.60s", f)
	}
	for i := range 9 {
		want := fmt.Sprintf("[s%d]crop=%d:%d:%d:%d,", i, tw, th, (i%3)*tw, (i/3)*th)
		if !strings.Contains(f, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Count(f, "zoompan") != 9 || strings.Count(f, p.MotionFilter(tw, th, 120)) != 9 {
		t.Error("every tile should get the same motion")
	}
	if strings.Count(f, "vignette") != 1 || !strings.Contains(f, "xstack=inputs=9:grid=3x3,scale=1920:1080,setsar=1,") {
		t.Errorf("grade should run once over the stacked grid: %s", f)
	}
	if strings.HasSuffix(f, "]") {
		t.Error("output must be unlabelled")
	}
	if MosaicFilter(p, 1920, 1080, 1, 120) != p.BuildFilter(1920, 1080, 120) {
		t.Error("1x1 should be the plain filter")
	}
}
