package layout

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/nothinglefttolearn/nltl-animate/internal/motion"
)

func tinter(t *testing.T, seed uint64) motion.Tinter {
	t.Helper()
	m, ok := motion.Get("shifting")
	if !ok {
		t.Fatal("no shifting motion")
	}
	return m.Tinter(seed)
}

// tinted is src with cast applied, as the layouts should draw it.
func tinted(src color.RGBA, c motion.Cast) color.RGBA {
	return color.RGBA{R: to8(float32(float64(src.R) * c[0])), G: to8(float32(float64(src.G) * c[1])), B: to8(float32(float64(src.B) * c[2])), A: src.A}
}

func frameAt(buf []byte, f, w, h int) *image.RGBA {
	n := w * h * 4
	return &image.RGBA{Pix: buf[f*n : (f+1)*n], Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
}

func TestTintTable(t *testing.T) {
	tt := newTintTable(motion.Cast{1.1, 0.95, 1.5})
	src := []byte{100, 200, 250, 255, 0, 0, 0, 0}
	dst := make([]byte, len(src))
	tt.copy(dst, src)
	want := []byte{110, 190, 255, 255, 0, 0, 0, 0} // blue clamps; alpha untouched
	if !bytes.Equal(dst, want) {
		t.Errorf("got %v, want %v", dst, want)
	}
}

// A single image with a tint becomes a cycle of tinted copies of it.
func TestCycleTintsEachAppearance(t *testing.T) {
	gray := color.RGBA{R: 128, G: 128, B: 128, A: 255}
	for _, loop := range []bool{false, true} {
		c, err := NewCycle([]*image.RGBA{solid(4, 2, gray)}, 2, 3)
		if err != nil {
			t.Fatal(err)
		}
		c.Loop = loop
		c.Tint = tinter(t, 3)
		const frames = 8 // 4 appearances of 2 frames each
		var buf bytes.Buffer
		if err := c.WriteFrames(&buf, frames); err != nil {
			t.Fatal(err)
		}
		period := 0
		if loop {
			period = frames / 2
		}
		var prev color.RGBA
		for f := range frames {
			got := frameAt(buf.Bytes(), f, 4, 2).RGBAAt(3, 1)
			if want := tinted(gray, c.Tint.Cast(f/2, period)); got != want {
				t.Fatalf("loop %v frame %d = %v, want %v", loop, f, got, want)
			}
			if f%2 == 0 && f > 0 && got == prev {
				t.Errorf("loop %v frame %d: tint did not change on the switch", loop, f)
			}
			prev = got
		}
	}
}

// With several images, the image and the tint both change on every switch.
func TestCycleTintWithSeveralImages(t *testing.T) {
	cols := []color.RGBA{{R: 200, G: 100, B: 50, A: 255}, {R: 50, G: 100, B: 200, A: 255}}
	c, err := NewCycle([]*image.RGBA{solid(2, 2, cols[0]), solid(2, 2, cols[1])}, 1, 9)
	if err != nil {
		t.Fatal(err)
	}
	c.Tint = tinter(t, 9)
	var buf bytes.Buffer
	if err := c.WriteFrames(&buf, 6); err != nil {
		t.Fatal(err)
	}
	for f, idx := range Sequence(2, 6, 1, 9) {
		if got, want := frameAt(buf.Bytes(), f, 2, 2).RGBAAt(0, 0), tinted(cols[idx], c.Tint.Cast(f, 0)); got != want {
			t.Errorf("frame %d = %v, want image %d tinted %v", f, got, idx, want)
		}
	}
}

// Every tile is its own appearance, even when every tile shows the same image.
func TestSameMosaicTintsEachTile(t *testing.T) {
	gray := color.RGBA{R: 128, G: 128, B: 128, A: 255}
	m, err := NewSameMosaic([]*image.RGBA{solid(2, 2, gray)}, 2, 3, 4)
	if err != nil {
		t.Fatal(err)
	}
	m.Tint = tinter(t, 4)
	m.Loop = true
	const frames, tiles = 6, 4 // 2 steps
	var buf bytes.Buffer
	if err := m.WriteFrames(&buf, frames); err != nil {
		t.Fatal(err)
	}
	for f := range frames {
		img := frameAt(buf.Bytes(), f, 4, 4)
		seen := map[color.RGBA]bool{}
		for tile := range tiles {
			got := img.RGBAAt((tile%2)*2+1, (tile/2)*2+1)
			if want := tinted(gray, m.Tint.Cast(f/3*tiles+tile, 2*tiles)); got != want {
				t.Fatalf("frame %d tile %d = %v, want %v", f, tile, got, want)
			}
			seen[got] = true
		}
		if len(seen) != tiles {
			t.Errorf("frame %d: %d different colors across %d tiles", f, len(seen), tiles)
		}
	}
}

// With --tiles same and several images, every tile shows the same image at
// once, each with its own cast.
func TestSameMosaicShowsOneImageAtATime(t *testing.T) {
	cols := []color.RGBA{{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255}}
	var imgs []*image.RGBA
	for _, c := range cols {
		imgs = append(imgs, solid(2, 2, c))
	}
	m, err := NewSameMosaic(imgs, 2, 1, 8)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := m.WriteFrames(&buf, 5); err != nil {
		t.Fatal(err)
	}
	for f, idx := range Sequence(3, 5, 1, 8) {
		img := frameAt(buf.Bytes(), f, 4, 4)
		for tile := range 4 {
			if got := img.RGBAAt((tile%2)*2, (tile/2)*2); got != cols[idx] {
				t.Errorf("frame %d tile %d = %v, want image %d", f, tile, got, idx)
			}
		}
	}
}

func TestMixedMosaicTintsEachTile(t *testing.T) {
	cols := []color.RGBA{{R: 200, G: 100, B: 50, A: 255}, {R: 50, G: 100, B: 200, A: 255}, {R: 100, G: 200, B: 50, A: 255}}
	var imgs []*image.RGBA
	for _, c := range cols {
		imgs = append(imgs, solid(2, 2, c))
	}
	m, err := NewMosaic(imgs, 2, 2, 5)
	if err != nil {
		t.Fatal(err)
	}
	m.Tint = tinter(t, 5)
	var buf bytes.Buffer
	if err := m.WriteFrames(&buf, 6); err != nil {
		t.Fatal(err)
	}
	steps := MosaicSequence(3, 4, 3, 5)
	for f := range 6 {
		img := frameAt(buf.Bytes(), f, 4, 4)
		for tile, idx := range steps[f/2] {
			got := img.RGBAAt((tile%2)*2, (tile/2)*2)
			if want := tinted(cols[idx], m.Tint.Cast(f/2*4+tile, 0)); got != want {
				t.Errorf("frame %d tile %d = %v, want image %d tinted %v", f, tile, got, idx, want)
			}
		}
	}
}

func TestNewSameMosaicErrors(t *testing.T) {
	a := solid(4, 4, color.RGBA{A: 255})
	for name, err := range map[string]error{
		"no images":  must(NewSameMosaic(nil, 2, 1, 1)),
		"zero grid":  must(NewSameMosaic([]*image.RGBA{a}, 0, 1, 1)),
		"zero hold":  must(NewSameMosaic([]*image.RGBA{a}, 2, 0, 1)),
		"size mixup": must(NewSameMosaic([]*image.RGBA{a, solid(5, 4, color.RGBA{})}, 2, 1, 1)),
	} {
		if err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestDrawBilinearTint(t *testing.T) {
	gray := color.RGBA{R: 100, G: 100, B: 100, A: 255}
	dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
	drawBilinear(dst, solid(8, 8, gray), -2, -2, 8, 8, 1, [3]float32{1.2, 1, 0.5})
	if got, want := dst.RGBAAt(1, 1), (color.RGBA{R: 120, G: 100, B: 50, A: 255}); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Each car keeps its own cast for its whole trip, and neighbors differ.
func TestTrainTintsEachCar(t *testing.T) {
	o := DefaultTrainOptions()
	o.Copies = 6
	o.Tint = tinter(t, 11)
	tr, err := NewTrain([]image.Image{solid(32, 32, color.RGBA{R: 128, G: 128, B: 128, A: 255})}, 96, 96, o)
	if err != nil {
		t.Fatal(err)
	}
	car := map[int]motion.Cast{}
	for f := range 40 {
		for _, c := range tr.copies(float64(f) / 30) {
			if c.cast != tr.opts.Tint.Cast(c.car, 0) {
				t.Fatalf("frame %d car %d: cast %v, want Cast(%d)", f, c.car, c.cast, c.car)
			}
			car[c.car] = c.cast
		}
	}
	for n, c := range car {
		if next, ok := car[n+1]; ok && next == c {
			t.Errorf("cars %d and %d share a cast", n, n+1)
		}
	}
	if len(car) <= o.Copies {
		t.Errorf("saw %d cars in 40 frames; want new cars entering", len(car))
	}

	plain := DefaultTrainOptions()
	plain.Copies = 6
	untinted, _ := NewTrain([]image.Image{solid(32, 32, color.RGBA{R: 128, G: 128, B: 128, A: 255})}, 96, 96, plain)
	a, b := image.NewRGBA(image.Rect(0, 0, 96, 96)), image.NewRGBA(image.Rect(0, 0, 96, 96))
	tr.RenderFrame(a, 10)
	untinted.RenderFrame(b, 10)
	if bytes.Equal(a.Pix, b.Pix) {
		t.Error("tinted frame looks the same as untinted")
	}
}

func TestTrainTintLoopIsSeamless(t *testing.T) {
	imgs := []image.Image{ring(32), solid(40, 32, color.RGBA{G: 200, A: 255})}
	o := DefaultTrainOptions()
	o.BPM, o.Copies = 138, 6
	o.Tint = tinter(t, 2)
	o.LoopFrames = TrainLoopFrames(o, len(imgs), 90)
	tr, err := NewTrain(imgs, 96, 96, o)
	if err != nil {
		t.Fatal(err)
	}
	first, loop := image.NewRGBA(image.Rect(0, 0, 96, 96)), image.NewRGBA(image.Rect(0, 0, 96, 96))
	tr.RenderFrame(first, 0)
	tr.RenderFrame(loop, o.LoopFrames)
	if !bytes.Equal(first.Pix, loop.Pix) {
		t.Errorf("frame %d differs from frame 0", o.LoopFrames)
	}
}
