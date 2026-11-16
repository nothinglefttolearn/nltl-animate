package style

import (
	"fmt"
	"strings"

	"github.com/nothinglefttolearn/nltl-clip/internal/preset"
)

// MaxGrid is the largest supported grid size (MaxGrid x MaxGrid tiles).
const MaxGrid = 12

// GridFilter tiles the input n x n. The preset's motion runs once at tile
// size and is duplicated, so every tile moves in sync; the grade runs once
// over the assembled frame so vignette and grain span the whole picture.
func GridFilter(p preset.Preset, width, height, n, bpm int) string {
	if n <= 1 {
		return p.BuildFilter(width, height, bpm)
	}
	cw, ch := even(width/n), even(height/n)
	tiles := n * n

	var labels strings.Builder
	for i := range tiles {
		fmt.Fprintf(&labels, "[t%d]", i)
	}

	return fmt.Sprintf("%s,split=%d%s;%sxstack=inputs=%d:grid=%dx%d,scale=%d:%d,setsar=1,%s",
		p.MotionFilter(cw, ch, bpm),
		tiles, labels.String(),
		labels.String(), tiles, n, n,
		width, height,
		p.GradeFilter(bpm),
	)
}

// even rounds down to an even number (libx264 + yuv420p need even sizes),
// with a minimum of 2.
func even(v int) int {
	return max(2, v&^1)
}
