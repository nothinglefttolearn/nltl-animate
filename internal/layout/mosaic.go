package layout

import (
	"errors"
	"fmt"
	"image"
	"io"
	"math/rand/v2"
)

// Mosaic is an n x n grid in which every tile shows its own image sequence.
// All tiles switch together every hold frames; at each switch every tile
// moves to a different image than it showed before, and, when there are at
// least as many images as tiles, no two tiles show the same image at once.
// It implements ffmpeg.FrameSource; frames are n tiles wide and high, with
// no gaps (see MosaicFilter for the per-tile motion).
type Mosaic struct {
	// Loop makes the grid repeat after one step per image: see
	// MosaicLoopSequence.
	Loop bool

	n      int
	tw, th int
	images []*image.RGBA
	hold   int
	seed   uint64
}

// NewMosaic takes images that are already the tile size (see frames.Cover).
func NewMosaic(imgs []*image.RGBA, n, hold int, seed uint64) (*Mosaic, error) {
	if len(imgs) < 2 {
		return nil, errors.New("mixed tiles need at least 2 images")
	}
	if n < 1 {
		return nil, errors.New("grid must be at least 1x1")
	}
	if hold < 1 {
		return nil, errors.New("hold must be at least 1 frame")
	}
	b := imgs[0].Bounds()
	for i, img := range imgs {
		if img.Bounds() != b || img.Stride != 4*b.Dx() {
			return nil, fmt.Errorf("mosaic image %d is %v, want tightly packed %v", i, img.Bounds(), b)
		}
	}
	return &Mosaic{n: n, tw: b.Dx(), th: b.Dy(), images: imgs, hold: hold, seed: seed}, nil
}

func (m *Mosaic) Size() (int, int) { return m.n * m.tw, m.n * m.th }

func (m *Mosaic) WriteFrames(w io.Writer, n int) error {
	fw, fh := m.Size()
	frame := image.NewRGBA(image.Rect(0, 0, fw, fh))
	var steps [][]int
	if m.Loop {
		steps = MosaicLoopSequence(len(m.images), m.n*m.n, m.seed)
	} else {
		steps = MosaicSequence(len(m.images), m.n*m.n, (n+m.hold-1)/m.hold, m.seed)
	}
	for i := range n {
		if i%m.hold == 0 {
			m.compose(frame, steps[(i/m.hold)%len(steps)])
		}
		if _, err := w.Write(frame.Pix); err != nil {
			return err
		}
	}
	return nil
}

// compose draws each tile's image into frame, row by row.
func (m *Mosaic) compose(frame *image.RGBA, tiles []int) {
	rowBytes := 4 * m.tw
	for t, idx := range tiles {
		src := m.images[idx]
		x0, y0 := (t%m.n)*m.tw, (t/m.n)*m.th
		for y := range m.th {
			d := frame.PixOffset(x0, y0+y)
			copy(frame.Pix[d:d+rowBytes], src.Pix[y*src.Stride:y*src.Stride+rowBytes])
		}
	}
}

// MosaicSequence returns, for each of nSteps steps, the image index of each
// of nTiles tiles. A tile never keeps its image from one step to the next.
// Images are dealt from shuffled decks, so they are spread as evenly as
// possible across the tiles at every step: all different when
// nImages >= nTiles. The same seed always gives the same sequence.
func MosaicSequence(nImages, nTiles, nSteps int, seed uint64) [][]int {
	return mosaicSteps(nImages, nTiles, nSteps, false, seed)
}

// MosaicLoopSequence is MosaicSequence for a clip that loops: nImages steps,
// after which it starts again from the first. The last step also differs
// from the first in every tile, so the loop point looks like any other
// switch.
func MosaicLoopSequence(nImages, nTiles int, seed uint64) [][]int {
	return mosaicSteps(nImages, nTiles, nImages, true, seed)
}

func mosaicSteps(nImages, nTiles, nSteps int, loop bool, seed uint64) [][]int {
	r := rand.New(rand.NewPCG(seed, 0x6d6f7361)) //nolint:gosec // visual randomness, not security
	steps := make([][]int, nSteps)
	var prev []int
	for s := range steps {
		var next []int
		if loop && s > 0 && s == nSteps-1 {
			next = steps[0]
		}
		steps[s] = dealTiles(r, nImages, nTiles, prev, next)
		prev = steps[s]
	}
	return steps
}

// dealTiles deals images to tiles from shuffled decks, then repairs any tile
// that drew the image it shows in the step before (prev) or after (next) by
// swapping with a tile that can take it. Either may be nil.
func dealTiles(r *rand.Rand, nImages, nTiles int, prev, next []int) []int {
	deal := make([]int, 0, nTiles+nImages)
	for len(deal) < nTiles {
		deal = append(deal, r.Perm(nImages)...)
	}
	deal = deal[:nTiles]
	clashes := func(tile, img int) bool {
		return (prev != nil && prev[tile] == img) || (next != nil && next[tile] == img)
	}
	for i := range deal {
		if !clashes(i, deal[i]) {
			continue
		}
		swapped := false
		for _, j := range r.Perm(nTiles) {
			if j != i && !clashes(i, deal[j]) && !clashes(j, deal[i]) {
				deal[i], deal[j] = deal[j], deal[i]
				swapped = true
				break
			}
		}
		if !swapped {
			// No swap works (e.g. a single tile): pick any image that fits.
			// There is always one: next is only set with 3+ images, or
			// equal to prev.
			var fits []int
			for img := range nImages {
				if !clashes(i, img) {
					fits = append(fits, img)
				}
			}
			deal[i] = fits[r.IntN(len(fits))]
		}
	}
	return deal
}
