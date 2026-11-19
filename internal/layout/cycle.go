package layout

import (
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"math/rand/v2"

	"github.com/nothinglefttolearn/nltl-animate/internal/motion"
	"github.com/nothinglefttolearn/nltl-animate/internal/video"
)

// Cycle shows a random one of its images on every frame (or every Hold
// frames), never the same image twice in a row. With a tint, every switch is
// a new appearance with its own cast, so even a single image changes color
// on every switch. It implements ffmpeg.FrameSource.
type Cycle struct {
	// Loop makes the sequence repeat: see LoopSequence. The casts then
	// repeat with the clip.
	Loop bool
	// Tint gives each switch its own cast.
	Tint motion.Tinter

	w, h   int
	images []*image.RGBA
	hold   int
	seed   uint64
}

// NewCycle takes frames that are already the output size (see
// frames.Cover), so callers can decode and shrink large source images one at
// a time. Memory is about w*h*4 bytes per image.
func NewCycle(imgs []*image.RGBA, hold int, seed uint64) (*Cycle, error) {
	if len(imgs) < 1 {
		return nil, errors.New("cycle needs at least 1 image")
	}
	if hold < 1 {
		return nil, errors.New("hold must be at least 1 frame")
	}
	b, err := sameSize("cycle", imgs)
	if err != nil {
		return nil, err
	}
	return &Cycle{w: b.Dx(), h: b.Dy(), images: imgs, hold: hold, seed: seed}, nil
}

func (c *Cycle) Size() (int, int) { return c.w, c.h }

func (c *Cycle) WriteFrames(w io.Writer, n int) error {
	seq := Sequence
	if c.Loop {
		seq = LoopSequence
	}
	var tinted []byte
	if c.Tint.On() {
		tinted = make([]byte, c.w*c.h*4)
	}
	period := castPeriod(c.Loop, n, c.hold, 1)
	for i, idx := range seq(len(c.images), n, c.hold, c.seed) {
		frame := c.images[idx].Pix
		if tinted != nil {
			if i%c.hold == 0 {
				newTintTable(c.Tint.Cast(i/c.hold, period)).copy(tinted, frame)
			}
			frame = tinted
		}
		if _, err := w.Write(frame); err != nil {
			return err
		}
	}
	return nil
}

// sameSize checks that imgs are all the size of the first and tightly
// packed, and returns that size.
func sameSize(what string, imgs []*image.RGBA) (image.Rectangle, error) {
	b := imgs[0].Bounds()
	for i, img := range imgs {
		if img.Bounds() != b || img.Stride != 4*b.Dx() {
			return b, fmt.Errorf("%s image %d is %v, want tightly packed %v", what, i, img.Bounds(), b)
		}
	}
	return b, nil
}

// Sequence returns the image index for each of nFrames frames. Each pick is
// held for hold frames and differs from the previous pick. The same seed
// always gives the same sequence.
func Sequence(nImages, nFrames, hold int, seed uint64) []int {
	r := rand.New(rand.NewPCG(seed, 0x6e6c746c)) //nolint:gosec // visual randomness, not security
	seq := make([]int, nFrames)
	if nImages < 2 {
		return seq // only image 0
	}
	prev := -1
	for i := range seq {
		if i%hold == 0 {
			var next int
			if prev < 0 {
				next = r.IntN(nImages)
			} else {
				next = r.IntN(nImages - 1)
				if next >= prev {
					next++ // skip prev: uniform over the other images
				}
			}
			prev = next
		}
		seq[i] = prev
	}
	return seq
}

// LoopSequence is Sequence for a clip that loops: the images are shuffled
// once and shown in that order, each for hold frames, over and over. One pass
// is nImages*hold frames. Every image appears once per pass, so the last
// differs from the first and the loop point looks like any other switch.
func LoopSequence(nImages, nFrames, hold int, seed uint64) []int {
	r := rand.New(rand.NewPCG(seed, 0x6e6c746c)) //nolint:gosec // visual randomness, not security
	order := r.Perm(nImages)
	seq := make([]int, nFrames)
	for i := range seq {
		seq[i] = order[(i/hold)%nImages]
	}
	return seq
}

// HoldFromBeats converts a hold length in beats to frames (at least 1).
func HoldFromBeats(beats float64, bpm int) int {
	return max(1, int(math.Round(beats*60/float64(bpm)*video.FPS)))
}
