package layout

import (
	"fmt"
	"strings"

	"github.com/nothinglefttolearn/nltl-animate/internal/grade"
	"github.com/nothinglefttolearn/nltl-animate/internal/motion"
)

// MaxGrid is the largest supported grid size (MaxGrid x MaxGrid tiles).
const MaxGrid = 12

// Filter is the treatment for one picture filling width x height: the
// motion's breath and shake, then the grade with the motion's pulse.
func Filter(m motion.Motion, g grade.Grade, width, height, bpm int) string {
	return m.MoveFilter(width, height, bpm) + "," + g.Filter(m.PulseExpr(bpm))
}

// GridFilter tiles the input n x n. The motion's breath and shake run once at
// tile size and are duplicated, so every tile moves in sync; the pulse and
// grade run once over the assembled frame so vignette and grain span the
// whole picture.
func GridFilter(m motion.Motion, g grade.Grade, width, height, n, bpm int) string {
	if n <= 1 {
		return Filter(m, g, width, height, bpm)
	}
	cw, ch := GridTileSize(width, height, n)
	tiles := n * n

	var labels strings.Builder
	for i := range tiles {
		fmt.Fprintf(&labels, "[t%d]", i)
	}

	return fmt.Sprintf("%s,split=%d%s;%sxstack=inputs=%d:grid=%dx%d,scale=%d:%d,setsar=1,%s",
		m.MoveFilter(cw, ch, bpm),
		tiles, labels.String(),
		labels.String(), tiles, n, n,
		width, height,
		g.Filter(m.PulseExpr(bpm)),
	)
}

// MosaicFilter takes a frame made of n x n tiles of tw x th each (a Mosaic,
// with tile size from GridTileSize) and gives every tile the motion's breath
// and shake separately, with the same parameters, so the tiles move in sync
// while showing different images. The pulse and grade run once over the
// assembled frame.
func MosaicFilter(m motion.Motion, g grade.Grade, width, height, n, bpm int) string {
	if n <= 1 {
		return Filter(m, g, width, height, bpm)
	}
	tw, th := GridTileSize(width, height, n)
	tiles := n * n
	move := m.MoveFilter(tw, th, bpm)

	var split, chains, stacked strings.Builder
	for i := range tiles {
		fmt.Fprintf(&split, "[s%d]", i)
		fmt.Fprintf(&chains, "[s%d]crop=%d:%d:%d:%d,%s[t%d];", i, tw, th, (i%n)*tw, (i/n)*th, move, i)
		fmt.Fprintf(&stacked, "[t%d]", i)
	}
	return fmt.Sprintf("split=%d%s;%s%sxstack=inputs=%d:grid=%dx%d,scale=%d:%d,setsar=1,%s",
		tiles, split.String(),
		chains.String(),
		stacked.String(), tiles, n, n,
		width, height,
		g.Filter(m.PulseExpr(bpm)),
	)
}

// GridTileSize is the size of one tile in an n x n grid filling
// width x height. Tiles are even-sized, so the grid may be a few pixels
// smaller than the frame before the final scale.
func GridTileSize(width, height, n int) (int, int) {
	return even(width / n), even(height / n)
}

// even rounds down to an even number (libx264 + yuv420p need even sizes),
// with a minimum of 2.
func even(v int) int {
	return max(2, v&^1)
}
