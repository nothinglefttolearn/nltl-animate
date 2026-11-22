package layout

import (
	"bytes"
	"cmp"
	"image"
	"image/color"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/nothinglefttolearn/nltl-animate/internal/video"
)

func newTestTrain(t *testing.T, img image.Image, w, h int, mod func(*TrainOptions)) *Train {
	t.Helper()
	o := DefaultTrainOptions()
	if mod != nil {
		mod(&o)
	}
	tr, err := NewTrain([]image.Image{img}, w, h, o)
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
			left, _, width, _ := tr.screenRect(tr.near, tr.aspect)
			if left+width >= 0 {
				t.Errorf("vanish=%v aspect=%.2f: copy at the near plane still visible (right edge %.1f)",
					vanish, tr.aspect, left+width)
			}
			// And the farthest copy is right of the vanishing point's left side
			// only by its own width: the track recedes toward cx.
			l2, _, w2, _ := tr.screenRect(tr.near+tr.length, tr.aspect)
			if l2+w2 > tr.cx || l2+w2 < tr.cx-w2*2 {
				t.Errorf("far copy at %.1f..%.1f should end just left of cx=%.1f", l2, l2+w2, tr.cx)
			}
		}
	}
}

func TestTrainCarsFollowTheLineup(t *testing.T) {
	imgs := []image.Image{
		solid(32, 32, color.RGBA{R: 255, A: 255}),
		solid(64, 32, color.RGBA{G: 255, A: 255}),
		solid(16, 32, color.RGBA{B: 255, A: 255}),
	}
	for _, copies := range []int{24, 7, 2} { // multiple of 3, not a multiple, fewer copies than images
		o := DefaultTrainOptions()
		o.Copies, o.Seed = copies, 42
		tr, err := NewTrain(imgs, 1080, 1080, o)
		if err != nil {
			t.Fatal(err)
		}
		if tr.aspect != 2 {
			t.Fatalf("track should be offset by the widest image, aspect=%v", tr.aspect)
		}
		prevNearest := 0
		for i := range 400 {
			cs := tr.copies(float64(i) * 0.05)
			for j := len(cs) - 1; j >= 0; j-- {
				if want := tr.imageFor(cs[j].car); cs[j].img != want {
					t.Fatalf("copies=%d: car %d shows image %d, want %d", copies, cs[j].car, cs[j].img, want)
				}
				// Near to far, each copy is the next car.
				if j > 0 && cs[j-1].car != cs[j].car+1 {
					t.Fatalf("copies=%d: car behind car %d is %d", copies, cs[j].car, cs[j-1].car)
				}
			}
			// When the nearest car wraps away, the next car takes its place.
			nearest := cs[len(cs)-1].car
			if i > 0 && nearest != prevNearest && nearest != prevNearest+1 {
				t.Fatalf("copies=%d: nearest jumped from car %d to %d", copies, prevNearest, nearest)
			}
			prevNearest = nearest
		}
	}
}

// trainOrder returns the images of cars from..to-1.
func trainOrder(t *testing.T, nImages int, seed uint64, loopFrames, from, to int) []int {
	t.Helper()
	imgs := make([]image.Image, nImages)
	for i := range imgs {
		imgs[i] = solid(8, 8, color.RGBA{R: uint8(i), A: 255})
	}
	o := DefaultTrainOptions()
	o.Seed, o.LoopFrames = seed, loopFrames
	tr, err := NewTrain(imgs, 64, 64, o)
	if err != nil {
		t.Fatal(err)
	}
	order := make([]int, 0, to-from)
	for n := from; n < to; n++ {
		order = append(order, tr.imageFor(n))
	}
	return order
}

func TestTrainShufflesImagesInPasses(t *testing.T) {
	for _, nImages := range []int{1, 2, 3, 4, 10} {
		for seed := range uint64(20) {
			order := trainOrder(t, nImages, seed, 0, -10*nImages, 10*nImages)
			for i := 0; i < len(order); i += nImages {
				pass := slices.Sorted(slices.Values(order[i : i+nImages]))
				for img, got := range pass {
					if got != img {
						t.Fatalf("%d images, seed %d: pass %v doesn't show each image once", nImages, seed, order[i:i+nImages])
					}
				}
			}
			for i := 1; i < len(order) && nImages > 1; i++ {
				if order[i] == order[i-1] {
					t.Fatalf("%d images, seed %d: image %d on two cars in a row at %d: %v", nImages, seed, order[i], i, order)
				}
			}
		}
	}
}

func TestTrainOrderIsRandomButReproducible(t *testing.T) {
	a := trainOrder(t, 10, 1, 0, 0, 50)
	if !slices.Equal(a, trainOrder(t, 10, 1, 0, 0, 50)) {
		t.Error("the same seed gave a different order")
	}
	if slices.Equal(a, trainOrder(t, 10, 2, 0, 0, 50)) {
		t.Error("another seed gave the same order")
	}
	if slices.Equal(a[:10], []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}) {
		t.Error("the first pass is in input order")
	}
	if slices.Equal(a[:10], a[10:20]) && slices.Equal(a[10:20], a[20:30]) {
		t.Error("every pass has the same order: passes should be shuffled anew")
	}
}

func TestTrainOrderRepeatsWithTheLoop(t *testing.T) {
	o := DefaultTrainOptions()
	for _, nImages := range []int{3, 5} {
		for _, minFrames := range []int{0, 400} { // one pass, several passes
			loop := TrainLoopFrames(o, nImages, minFrames)
			cars := int(math.Round(float64(loop) / video.FPS * o.PassesPerSecond()))
			order := trainOrder(t, nImages, 7, loop, -cars, 2*cars)
			if !slices.Equal(order[:cars], order[cars:2*cars]) || !slices.Equal(order[:cars], order[2*cars:]) {
				t.Errorf("%d images, %d frames: the order doesn't repeat every %d cars: %v", nImages, loop, cars, order)
			}
			for i := 1; i < len(order); i++ {
				if order[i] == order[i-1] {
					t.Errorf("%d images, %d frames: image %d twice in a row at the loop point: %v", nImages, loop, order[i], order)
				}
			}
		}
	}
}

func TestTrainMixedAspectsShareInnerEdge(t *testing.T) {
	tr, err := NewTrain([]image.Image{solid(32, 32, color.RGBA{A: 255}), solid(96, 32, color.RGBA{A: 255})},
		1080, 1080, DefaultTrainOptions())
	if err != nil {
		t.Fatal(err)
	}
	z := tr.near + 3
	l1, _, w1, _ := tr.screenRect(z, tr.imgs[0].aspect)
	l2, _, w2, _ := tr.screenRect(z, tr.imgs[1].aspect)
	if math.Abs((l1+w1)-(l2+w2)) > 1e-9 || math.Abs(w2-3*w1) > 1e-9 {
		t.Errorf("inner edges %v vs %v, widths %v vs %v", l1+w1, l2+w2, w1, w2)
	}
}

func TestTrainDropsUnusedMipLevels(t *testing.T) {
	// A wide image sits far from the viewer, so the nearest copy is only
	// ~177 px tall here: of the 1024..32 chain, only 256 and below are needed.
	tr := newTestTrain(t, solid(4096, 1024, color.RGBA{A: 255}), 320, 320, nil)
	maxHeight := tr.focal / tr.near
	top := float64(tr.imgs[0].mips[0].Bounds().Dy())
	if top < maxHeight || top >= 2*maxHeight {
		t.Errorf("largest kept level is %v px tall; want the smallest level >= %v", top, maxHeight)
	}

	// A source smaller than the largest drawn size is kept whole.
	small := newTestTrain(t, solid(64, 64, color.RGBA{A: 255}), 1080, 1080, nil)
	if got := small.imgs[0].mips[0].Bounds().Dy(); got != 64 {
		t.Errorf("small source should be kept, top level %d px tall", got)
	}
}

func TestNewTrainNeedsImages(t *testing.T) {
	if _, err := NewTrain(nil, 64, 64, DefaultTrainOptions()); err == nil {
		t.Error("expected an error with no images")
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
		t.Errorf("expected a red copy left of center, got %v", c)
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
	drawBilinear(dst, src, 0, 0, 40, 40, 1, noShade)
	if c := dst.RGBAAt(5, 20); c.R != 255 || c.B != 0 {
		t.Errorf("opaque half should cover background, got %v", c)
	}
	if c := dst.RGBAAt(35, 20); c != bg {
		t.Errorf("transparent half should show what is behind, got %v", c)
	}

	dst = solid(40, 40, bg)
	drawBilinear(dst, src, 0, 0, 40, 40, 0.5, noShade)
	if c := dst.RGBAAt(5, 20); c.R < 120 || c.R > 135 || c.B < 120 || c.B > 135 || c.A != 255 {
		t.Errorf("50%% opacity should blend evenly, got %v", c)
	}
}

func TestDrawBilinearClipsOffscreen(t *testing.T) {
	dst := solid(10, 10, color.RGBA{A: 255})
	src := solid(4, 4, color.RGBA{R: 255, A: 255})
	drawBilinear(dst, src, -1000, -1000, 5000, 5000, 1, noShade) // huge, mostly offscreen
	drawBilinear(dst, src, 50, 50, 5, 5, 1, noShade)             // fully offscreen
	if c := dst.RGBAAt(5, 5); c.R != 255 {
		t.Errorf("got %v", c)
	}
}

func TestMipChainAndSelection(t *testing.T) {
	tr := newTestTrain(t, solid(512, 256, color.RGBA{G: 255, A: 255}), 1080, 1080, nil)
	var heights []int
	for _, m := range tr.imgs[0].mips {
		heights = append(heights, m.Bounds().Dy())
	}
	if !slices.Equal(heights, []int{256, 128, 64, 32, 16}) {
		t.Errorf("mip heights = %v", heights)
	}
	if c := tr.imgs[0].mips[3].RGBAAt(1, 1); c.G != 255 {
		t.Errorf("mip color drifted: %v", c)
	}
	for target, want := range map[float64]int{1000: 256, 200: 256, 128: 128, 100: 128, 20: 32, 3: 16} {
		if got := tr.imgs[0].mipFor(target).Bounds().Dy(); got != want {
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

func TestTrainLoopFrames(t *testing.T) {
	o := DefaultTrainOptions() // 1.5 copies per second: one every 20 frames
	if got := TrainLoopFrames(o, 3, 0); got != 60 {
		t.Errorf("3 images: %d frames, want 60 (three copies)", got)
	}
	if got := TrainLoopFrames(o, 1, 90); got != 100 {
		t.Errorf("1 image, at least 90: %d frames, want 100 (five copies)", got)
	}
	if got := TrainLoopFrames(o, 2, 80); got != 80 {
		t.Errorf("2 images, at least 80: %d frames, want 80", got)
	}
	o.BPM = 138 // 13.04 frames per copy
	if got := TrainLoopFrames(o, 2, 0); got != 26 {
		t.Errorf("138 BPM, 2 images: %d frames, want 26", got)
	}
}

func TestTrainLoopIsSeamless(t *testing.T) {
	// 138 BPM does not divide into whole frames; the speed is nudged so
	// frame LoopFrames matches frame 0 exactly, images included.
	imgs := []image.Image{ring(32), solid(40, 32, color.RGBA{G: 200, A: 255}), solid(24, 32, color.RGBA{B: 200, A: 255})}
	o := DefaultTrainOptions()
	o.BPM, o.Copies = 138, 6
	o.LoopFrames = TrainLoopFrames(o, len(imgs), 0)
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
	mid := image.NewRGBA(image.Rect(0, 0, 96, 96))
	tr.RenderFrame(mid, o.LoopFrames/len(imgs))
	if bytes.Equal(first.Pix, mid.Pix) {
		t.Error("one copy later looks the same as frame 0: the images should have moved along")
	}
}
