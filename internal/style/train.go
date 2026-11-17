package style

import (
	"cmp"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"runtime"
	"slices"
	"sync"

	"github.com/nothinglefttolearn/nltl-animate/internal/frames"
	"github.com/nothinglefttolearn/nltl-animate/internal/video"
)

// TrainOptions controls the train fly-by. Zero values are not meaningful;
// start from DefaultTrainOptions.
type TrainOptions struct {
	Copies  int     // copies on the track at once
	Spacing float64 // depth gap between consecutive copies, in copy heights
	Speed   float64 // copies passing the viewer per second (ignored when BPM > 0)
	BPM     int     // when > 0, one copy passes every Beats beats
	Beats   float64
	Offset  float64 // gap between the track and the viewer, in widths of the widest image
	FOV     float64 // horizontal field of view, degrees
	Vanish  float64 // horizontal position of the vanishing point, 0 = left edge, 1 = right edge
	BG      color.RGBA
	Fog     float64 // 0 = none, 1 = the farthest copies fully dissolve into BG

	// LoopFrames, when > 0, makes the train repeat exactly every LoopFrames
	// frames: the speed is nudged so a whole number of image cycles pass in
	// that time. See TrainLoopFrames.
	LoopFrames int
}

func DefaultTrainOptions() TrainOptions {
	return TrainOptions{
		Copies:  24,
		Spacing: 1.0,
		Speed:   1.5,
		Beats:   1,
		Offset:  0.25,
		FOV:     60,
		Vanish:  0.5,
		BG:      color.RGBA{A: 255},
		Fog:     0.6,
	}
}

func (o TrainOptions) Validate() error {
	var errs []error
	if o.Copies < 1 || o.Copies > 200 {
		errs = append(errs, fmt.Errorf("copies must be between 1 and 200, got %d", o.Copies))
	}
	if o.Spacing <= 0 {
		errs = append(errs, fmt.Errorf("spacing must be > 0, got %g", o.Spacing))
	}
	if o.BPM > 0 && o.Beats <= 0 {
		errs = append(errs, fmt.Errorf("beats must be > 0, got %g", o.Beats))
	}
	if o.BPM <= 0 && o.Speed <= 0 {
		errs = append(errs, fmt.Errorf("speed must be > 0, got %g", o.Speed))
	}
	if o.Offset < 0.01 {
		// At 0 the copies' inner edge sits on the line of sight and never
		// leaves the screen.
		errs = append(errs, fmt.Errorf("offset must be >= 0.01, got %g", o.Offset))
	}
	if o.FOV < 10 || o.FOV > 150 {
		errs = append(errs, fmt.Errorf("fov must be between 10 and 150 degrees, got %g", o.FOV))
	}
	if o.Vanish < 0.1 || o.Vanish > 1 {
		errs = append(errs, fmt.Errorf("vanish must be between 0.1 and 1, got %g", o.Vanish))
	}
	if o.Fog < 0 || o.Fog > 1 {
		errs = append(errs, fmt.Errorf("fog must be between 0 and 1, got %g", o.Fog))
	}
	if o.LoopFrames < 0 {
		errs = append(errs, fmt.Errorf("loop frames must be >= 0, got %d", o.LoopFrames))
	}
	return errors.Join(errs...)
}

// PassesPerSecond is how many copies pass the viewer each second.
func (o TrainOptions) PassesPerSecond() float64 {
	if o.BPM > 0 {
		return float64(o.BPM) / 60 / o.Beats
	}
	return o.Speed
}

// TrainLoopFrames is the length of a seamless loop of a train of nImages:
// the time for nImages copies to pass (the cars then show the same images in
// the same places again), repeated until it lasts at least minFrames, and
// rounded to whole frames. Pass it as LoopFrames.
func TrainLoopFrames(o TrainOptions, nImages, minFrames int) int {
	perSecond := o.PassesPerSecond()
	cycleFrames := float64(nImages) * video.FPS / perSecond
	cycles := max(1, math.Ceil(float64(minFrames)/cycleFrames-1e-9))
	return max(1, int(math.Round(cycles*cycleFrames)))
}

// fadeInFraction is the far share of the track over which copies fade in,
// so they never pop into existence.
const fadeInFraction = 0.15

// Train renders copies of one or more images on a straight track to the
// viewer's left, travelling towards and past the camera. With several images,
// consecutive cars show them in turn (0, 1, 2, ..., 0, 1, ...), so a
// different image passes on every step. It implements ffmpeg.FrameSource.
//
// World space: pinhole camera at the origin looking down +z, x right, y down.
// Each copy is a camera-facing billboard 1 unit tall and as wide as its
// image's aspect ratio. All copies share the inner (right) edge x = innerX,
// so images of different shapes still line up along the track.
type Train struct {
	w, h int
	opts TrainOptions
	imgs []trainImage

	aspect   float64 // widest image's width / height; sets the track offset
	focal    float64 // pixels per world unit at z = 1
	cx       float64 // screen x of the vanishing point (lens shift)
	innerX   float64 // x of every copy's inner (right) edge
	near     float64 // depth at which a copy wraps to the back
	length   float64 // track length in depth: Copies * Spacing
	velocity float64 // world units per second
}

// trainImage is one image on the train, ready to draw.
type trainImage struct {
	mips   []*image.RGBA // mips[0] is the largest kept level; each next is half size
	aspect float64       // width / height
}

// NewTrain builds a train from one or more images. Copies show the images
// in order, cycling through them along the lineup.
func NewTrain(imgs []image.Image, w, h int, opts TrainOptions) (*Train, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if len(imgs) == 0 {
		return nil, errors.New("train: no images")
	}

	t := &Train{w: w, h: h, opts: opts, imgs: make([]trainImage, len(imgs))}
	srcs := make([]*image.RGBA, len(imgs))
	for i, img := range imgs {
		src := frames.ToRGBA(img)
		if src.Bounds().Dx() < 1 || src.Bounds().Dy() < 1 {
			return nil, fmt.Errorf("train: image %d is empty", i+1)
		}
		srcs[i] = src
		t.imgs[i].aspect = float64(src.Bounds().Dx()) / float64(src.Bounds().Dy())
		t.aspect = max(t.aspect, t.imgs[i].aspect)
	}

	t.focal = float64(w) / 2 / math.Tan(opts.FOV*math.Pi/360)
	t.cx = opts.Vanish * float64(w)
	gap := opts.Offset * t.aspect
	t.innerX = -gap

	// A copy's inner (right) edge is at x = -gap. It is off the left of the
	// screen once cx - focal*gap/z < 0, i.e. z < focal*gap/cx. Wrap a
	// little closer than that so nothing visible ever pops.
	t.near = 0.9 * t.focal * gap / t.cx
	t.length = float64(opts.Copies) * opts.Spacing

	// No copy is ever drawn taller than it is at the near plane, so mip
	// levels above that size would never be sampled. Dropping them keeps
	// many large images from piling up in memory.
	maxHeight := t.focal / t.near
	for i, src := range srcs {
		mips := mipChain(src)
		for len(mips) > 1 && float64(mips[1].Bounds().Dy()) >= maxHeight {
			mips = mips[1:]
		}
		t.imgs[i].mips = mips
	}

	perSecond := opts.PassesPerSecond()
	if opts.LoopFrames > 0 {
		// Round to a whole number of image cycles in the loop, so frame
		// LoopFrames is identical to frame 0.
		loopSec := float64(opts.LoopFrames) / video.FPS
		n := float64(len(imgs))
		perSecond = max(1, math.Round(loopSec*perSecond/n)) * n / loopSec
	}
	t.velocity = perSecond * opts.Spacing
	return t, nil
}

func (t *Train) Size() (int, int) { return t.w, t.h }

// copyState is one copy's depth, opacity and image at a moment in time.
type copyState struct {
	z, alpha float64
	img      int // index into Train.imgs
}

// copies returns every copy at time sec, sorted far to near (draw order).
func (t *Train) copies(sec float64) []copyState {
	cs := make([]copyState, t.opts.Copies)
	for k := range cs {
		// d is the distance from the wrap point; it shrinks as time passes.
		// wraps counts how often this copy has gone round (negative, since
		// d decreases), which identifies the car on an endless train.
		p := float64(k)*t.opts.Spacing - sec*t.velocity
		wraps := math.Floor(p / t.length)
		d := p - wraps*t.length
		if d >= t.length { // floating-point edge
			d -= t.length
			wraps++
		}
		// Car n of the endless train; consecutive cars have consecutive n,
		// so they show consecutive images even across the wrap.
		n := k - int(wraps)*t.opts.Copies
		frac := d / t.length // 0 = nearest, 1 = farthest
		alpha := (1 - t.opts.Fog*frac) * clamp01((1-frac)/fadeInFraction)
		cs[k] = copyState{z: t.near + d, alpha: alpha, img: posMod(n, len(t.imgs))}
	}
	slices.SortFunc(cs, func(a, b copyState) int { return cmp.Compare(b.z, a.z) }) // far first
	return cs
}

// screenRect projects a copy of the given aspect ratio at depth z to pixel
// coordinates.
func (t *Train) screenRect(z, aspect float64) (left, top, width, height float64) {
	s := t.focal / z
	width, height = aspect*s, s
	left = t.cx + (t.innerX-aspect)*s
	top = float64(t.h)/2 - height/2
	return left, top, width, height
}

// RenderFrame draws frame i into dst, which must be w x h.
func (t *Train) RenderFrame(dst *image.RGBA, i int) {
	fillRGBA(dst, t.opts.BG)
	sec := float64(i) / video.FPS
	for _, c := range t.copies(sec) {
		if c.alpha <= 0 {
			continue
		}
		img := &t.imgs[c.img]
		left, top, width, height := t.screenRect(c.z, img.aspect)
		if left+width < 0 || left > float64(t.w) || height < 0.5 {
			continue
		}
		drawBilinear(dst, img.mipFor(height), left, top, width, height, c.alpha)
	}
}

// mipFor picks the smallest mip level at least as tall as the target, so
// bilinear sampling never minifies by more than 2x (which would shimmer).
func (ti *trainImage) mipFor(height float64) *image.RGBA {
	for i := len(ti.mips) - 1; i > 0; i-- {
		if float64(ti.mips[i].Bounds().Dy()) >= height {
			return ti.mips[i]
		}
	}
	return ti.mips[0]
}

// WriteFrames renders frames in parallel and writes them in order.
func (t *Train) WriteFrames(w io.Writer, n int) error {
	workers := runtime.NumCPU()
	inflight := make(chan struct{}, 2*workers) // bounds memory: frames rendered but not yet written
	done := make(chan struct{})
	defer close(done)

	pool := sync.Pool{New: func() any { return image.NewRGBA(image.Rect(0, 0, t.w, t.h)) }}
	slots := make([]chan *image.RGBA, n)
	for i := range slots {
		slots[i] = make(chan *image.RGBA, 1)
	}

	go func() {
		for i := range n {
			select {
			case inflight <- struct{}{}:
			case <-done:
				return
			}
			go func() {
				buf := pool.Get().(*image.RGBA) // the pool only ever holds *image.RGBA
				t.RenderFrame(buf, i)
				slots[i] <- buf
			}()
		}
	}()

	for i := range n {
		buf := <-slots[i]
		_, err := w.Write(buf.Pix)
		pool.Put(buf)
		<-inflight
		if err != nil {
			return err
		}
	}
	return nil
}

// mipChain returns src followed by successive 2x box-filtered halvings,
// down to about 16 px tall.
func mipChain(src *image.RGBA) []*image.RGBA {
	chain := []*image.RGBA{src}
	for cur := src; cur.Bounds().Dx() >= 2 && cur.Bounds().Dy() >= 32; {
		cur = halve(cur)
		chain = append(chain, cur)
	}
	return chain
}

func halve(src *image.RGBA) *image.RGBA {
	w, h := src.Bounds().Dx()/2, src.Bounds().Dy()/2
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			o00 := src.PixOffset(2*x, 2*y)
			o01 := o00 + src.Stride
			d := dst.PixOffset(x, y)
			for c := range 4 {
				sum := int(src.Pix[o00+c]) + int(src.Pix[o00+4+c]) + int(src.Pix[o01+c]) + int(src.Pix[o01+4+c])
				dst.Pix[d+c] = uint8((sum + 2) / 4) //nolint:gosec // average of four uint8
			}
		}
	}
	return dst
}

// drawBilinear composites src, scaled to the sub-pixel rectangle
// (left, top, width, height), over dst with an extra opacity. Both images
// are premultiplied RGBA. Samples outside src are transparent, which
// anti-aliases the copy's edges.
func drawBilinear(dst, src *image.RGBA, left, top, width, height, opacity float64) {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	db := dst.Bounds()
	x0 := max(db.Min.X, int(math.Floor(left)))
	x1 := min(db.Max.X, int(math.Ceil(left+width)))
	y0 := max(db.Min.Y, int(math.Floor(top)))
	y1 := min(db.Max.Y, int(math.Ceil(top+height)))
	if x0 >= x1 || y0 >= y1 {
		return
	}

	sx := float64(sw) / width
	sy := float64(sh) / height
	op := float32(opacity)

	texel := func(x, y int) (r, g, b, a float32) {
		if x < 0 || y < 0 || x >= sw || y >= sh {
			return 0, 0, 0, 0
		}
		o := y*src.Stride + x*4
		p := src.Pix[o : o+4 : o+4]
		return float32(p[0]), float32(p[1]), float32(p[2]), float32(p[3])
	}

	for y := y0; y < y1; y++ {
		v := (float64(y)+0.5-top)*sy - 0.5
		ty := int(math.Floor(v))
		fy := float32(v - float64(ty))
		row := y*dst.Stride - db.Min.Y*dst.Stride
		for x := x0; x < x1; x++ {
			u := (float64(x)+0.5-left)*sx - 0.5
			tx := int(math.Floor(u))
			fx := float32(u - float64(tx))

			r00, g00, b00, a00 := texel(tx, ty)
			r10, g10, b10, a10 := texel(tx+1, ty)
			r01, g01, b01, a01 := texel(tx, ty+1)
			r11, g11, b11, a11 := texel(tx+1, ty+1)
			w00, w10 := (1-fx)*(1-fy), fx*(1-fy)
			w01, w11 := (1-fx)*fy, fx*fy

			a := (a00*w00 + a10*w10 + a01*w01 + a11*w11) * op
			if a <= 0 {
				continue
			}
			r := (r00*w00 + r10*w10 + r01*w01 + r11*w11) * op
			g := (g00*w00 + g10*w10 + g01*w01 + g11*w11) * op
			b := (b00*w00 + b10*w10 + b01*w01 + b11*w11) * op

			// Premultiplied "over": dst = src + dst*(1-srcAlpha).
			k := 1 - a/255
			o := row + (x-db.Min.X)*4
			d := dst.Pix[o : o+4 : o+4]
			d[0] = to8(r + float32(d[0])*k)
			d[1] = to8(g + float32(d[1])*k)
			d[2] = to8(b + float32(d[2])*k)
			d[3] = to8(a + float32(d[3])*k)
		}
	}
}

func fillRGBA(dst *image.RGBA, c color.RGBA) {
	px := [4]uint8{c.R, c.G, c.B, c.A}
	for i := 0; i < len(dst.Pix); i += 4 {
		copy(dst.Pix[i:i+4], px[:])
	}
}

// to8 rounds and clamps a channel value to uint8.
func to8(v float32) uint8 {
	return uint8(min(255, max(0, v+0.5)))
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// posMod is a mod b in [0, b).
func posMod(a, b int) int { return ((a % b) + b) % b }
