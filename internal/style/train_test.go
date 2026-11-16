package style

import (
	"bytes"
	"cmp"
	"image"
	"image/color"
	"math"
	"slices"
	"strings"
	"testing"
)

func newTestTrain(t *testing.T, img image.Image, w, h int, mod func(*TrainOptions)) *Train {
	t.Helper()
	o := DefaultTrainOptions()
	if mod != nil {
		mod(&o)
	}
	tr, err := NewTrain(img, w, h, o)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func depths(cs []copyState) []float64 {
	z := make([]float64, len(cs))
	for i, c := range cs {
		z[i] = c.z
	}
	return z
}

func TestTrainCopiesFarToNearWithinTrack(t *testing.T) {
	tr := newTestTrain(t, ring(64), 1080, 1080, nil)
	for _, sec := range []float64{0, 0.37, 5, 123.4} {
		cs := tr.copies(sec)
		if len(cs) != 24 {
			t.Fatalf("got %d copies", len(cs))
		}
		z := depths(cs)
		if !slices.IsSortedFunc(z, func(a, b float64) int { return cmp.Compare(b, a) }) {
			t.Errorf("t=%v: copies not sorted far to near: %v", sec, z)
		}
		for _, c := range cs {
			if c.z < tr.near || c.z >= tr.near+tr.length {
				t.Errorf("t=%v: z=%v outside [%v, %v)", sec, c.z, tr.near, tr.near+tr.length)
			}
			if c.alpha < 0 || c.alpha > 1 {
				t.Errorf("alpha %v out of range", c.alpha)
			}
		}
	}
}

func TestTrainOneCopyPassesPerBeat(t *testing.T) {
	// 120 BPM, one copy per beat: the layout repeats every 0.5s, and in
	// between every copy has moved closer.
	tr := newTestTrain(t, ring(64), 1080, 1080, func(o *TrainOptions) { o.BPM = 120; o.Beats = 1 })
	for _, sec := range []float64{0, 1.3, 10} {
		a, b := depths(tr.copies(sec)), depths(tr.copies(sec+0.5))
		for i := range a {
			if math.Abs(a[i]-b[i]) > 1e-9 {
				t.Fatalf("t=%v: layout should repeat after one beat\n%v\n%v", sec, a, b)
			}
		}
		mid := depths(tr.copies(sec + 0.25))
		if slices.Equal(a, mid) {
			t.Errorf("t=%v: copies did not move within the beat", sec)
		}
	}
}

func TestTrainSpeedWithoutBPM(t *testing.T) {
	tr := newTestTrain(t, ring(64), 1080, 1080, func(o *TrainOptions) { o.Speed = 2 })
	a, b := depths(tr.copies(3)), depths(tr.copies(3.5))
	for i := range a {
		if math.Abs(a[i]-b[i]) > 1e-9 {
			t.Fatalf("2 copies/s should repeat every 0.5s\n%v\n%v", a, b)
		}
	}
}

func TestTrainExitsLeftBeforeWrapping(t *testing.T) {
	for _, vanish := range []float64{0.3, 0.5, 0.9} {
		for _, img := range []image.Image{ring(64), solid(300, 100, color.RGBA{A: 255}), solid(50, 200, color.RGBA{A: 255})} {
			tr := newTestTrain(t, img, 1080, 1920, func(o *TrainOptions) { o.Vanish = vanish })
			left, _, width, _ := tr.screenRect(tr.near)
			if left+width >= 0 {
				t.Errorf("vanish=%v aspect=%.2f: copy at the near plane still visible (right edge %.1f)",
					vanish, tr.aspect, left+width)
			}
			// And the farthest copy is right of the vanishing point's left side
			// only by its own width: the track recedes towards cx.
			l2, _, w2, _ := tr.screenRect(tr.near + tr.length)
			if l2+w2 > tr.cx || l2+w2 < tr.cx-w2*2 {
				t.Errorf("far copy at %.1f..%.1f should end just left of cx=%.1f", l2, l2+w2, tr.cx)
			}
		}
	}
}

func TestTrainFadesIn(t *testing.T) {
	tr := newTestTrain(t, ring(64), 1080, 1080, func(o *TrainOptions) { o.Fog = 0 })
	cs := tr.copies(0.01)
	if far := cs[0]; far.alpha >= 0.5 {
		t.Errorf("farthest copy should be mostly transparent, alpha=%v", far.alpha)
	}
	if near := cs[len(cs)-1]; near.alpha != 1 {
		t.Errorf("nearest copy should be opaque with no fog, alpha=%v", near.alpha)
	}
}

func TestTrainRenderFrame(t *testing.T) {
	bg := color.RGBA{B: 255, A: 255}
	tr := newTestTrain(t, solid(32, 32, color.RGBA{R: 255, A: 255}), 64, 36, func(o *TrainOptions) {
		o.BG, o.Fog = bg, 0
	})
	dst := image.NewRGBA(image.Rect(0, 0, 64, 36))
	tr.RenderFrame(dst, 0)

	// Everything right of the vanishing point is background.
	for y := range 36 {
		for x := 33; x < 64; x++ {
			if c := dst.RGBAAt(x, y); c != bg {
				t.Fatalf("pixel (%d,%d) = %v, want background", x, y, c)
			}
		}
	}
	// Left of it, at eye level, a copy is drawn.
	if c := dst.RGBAAt(20, 18); c.R < 200 {
		t.Errorf("expected a red copy left of centre, got %v", c)
	}
	for _, c := range []color.RGBA{dst.RGBAAt(0, 0), dst.RGBAAt(63, 35)} {
		if c.A != 255 {
			t.Errorf("frame must be fully opaque, got %v", c)
		}
	}
}

func TestDrawBilinearTransparencyAndOpacity(t *testing.T) {
	bg := color.RGBA{B: 255, A: 255}
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 { // left half opaque red, right half transparent
		for x := range 2 {
			src.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}

	dst := solid(40, 40, bg)
	drawBilinear(dst, src, 0, 0, 40, 40, 1)
	if c := dst.RGBAAt(5, 20); c.R != 255 || c.B != 0 {
		t.Errorf("opaque half should cover background, got %v", c)
	}
	if c := dst.RGBAAt(35, 20); c != bg {
		t.Errorf("transparent half should show what is behind, got %v", c)
	}

	dst = solid(40, 40, bg)
	drawBilinear(dst, src, 0, 0, 40, 40, 0.5)
	if c := dst.RGBAAt(5, 20); c.R < 120 || c.R > 135 || c.B < 120 || c.B > 135 || c.A != 255 {
		t.Errorf("50%% opacity should blend evenly, got %v", c)
	}
}

func TestDrawBilinearClipsOffscreen(t *testing.T) {
	dst := solid(10, 10, color.RGBA{A: 255})
	src := solid(4, 4, color.RGBA{R: 255, A: 255})
	drawBilinear(dst, src, -1000, -1000, 5000, 5000, 1) // huge, mostly offscreen
	drawBilinear(dst, src, 50, 50, 5, 5, 1)             // fully offscreen
	if c := dst.RGBAAt(5, 5); c.R != 255 {
		t.Errorf("got %v", c)
	}
}

func TestMipChainAndSelection(t *testing.T) {
	tr := newTestTrain(t, solid(512, 256, color.RGBA{G: 255, A: 255}), 1080, 1080, nil)
	var heights []int
	for _, m := range tr.mips {
		heights = append(heights, m.Bounds().Dy())
	}
	if !slices.Equal(heights, []int{256, 128, 64, 32, 16}) {
		t.Errorf("mip heights = %v", heights)
	}
	if c := tr.mips[3].RGBAAt(1, 1); c.G != 255 {
		t.Errorf("mip colour drifted: %v", c)
	}
	for target, want := range map[float64]int{1000: 256, 200: 256, 128: 128, 100: 128, 20: 32, 3: 16} {
		if got := tr.mipFor(target).Bounds().Dy(); got != want {
			t.Errorf("mipFor(%v) = %d tall, want %d", target, got, want)
		}
	}
}

func TestTrainWriteFramesMatchesSequentialRender(t *testing.T) {
	tr := newTestTrain(t, ring(48), 48, 32, nil)
	var buf bytes.Buffer
	if err := tr.WriteFrames(&buf, 40); err != nil {
		t.Fatal(err)
	}
	frameLen := 48 * 32 * 4
	if buf.Len() != 40*frameLen {
		t.Fatalf("wrote %d bytes, want %d", buf.Len(), 40*frameLen)
	}
	dst := image.NewRGBA(image.Rect(0, 0, 48, 32))
	for i := range 40 {
		tr.RenderFrame(dst, i)
		if !bytes.Equal(dst.Pix, buf.Bytes()[i*frameLen:(i+1)*frameLen]) {
			t.Fatalf("frame %d out of order or corrupted", i)
		}
	}
}

type failAfter struct{ n int }

func (f *failAfter) Write(p []byte) (int, error) {
	if f.n == 0 {
		return 0, errBoom
	}
	f.n--
	return len(p), nil
}

var errBoom = &boomErr{}

type boomErr struct{}

func (*boomErr) Error() string { return "boom" }

func TestTrainWriteFramesStopsOnError(t *testing.T) {
	tr := newTestTrain(t, ring(16), 16, 16, nil)
	if err := tr.WriteFrames(&failAfter{n: 3}, 1000); err != errBoom { //nolint:errorlint // sentinel identity
		t.Errorf("err = %v, want boom", err)
	}
}

func TestTrainOptionsValidate(t *testing.T) {
	if err := DefaultTrainOptions().Validate(); err != nil {
		t.Fatalf("defaults invalid: %v", err)
	}
	bad := DefaultTrainOptions()
	bad.Copies, bad.Spacing, bad.Speed, bad.Offset, bad.FOV, bad.Fog, bad.Vanish = 0, 0, 0, 0, 5, 2, 0
	err := bad.Validate()
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, s := range []string{"copies", "spacing", "speed", "offset", "fov", "fog", "vanish"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("missing %q in %v", s, err)
		}
	}
	bpm := DefaultTrainOptions()
	bpm.BPM, bpm.Speed, bpm.Beats = 120, 0, 0 // speed ignored with BPM, beats checked
	if err := bpm.Validate(); err == nil || strings.Contains(err.Error(), "speed") || !strings.Contains(err.Error(), "beats") {
		t.Errorf("BPM mode validation wrong: %v", err)
	}
}
