package layout

import "github.com/nothinglefttolearn/nltl-animate/internal/motion"

// tintTable is one cast as a lookup per channel, so applying it while
// copying an image costs three table lookups per pixel and no stored copy.
type tintTable [3][256]uint8

func newTintTable(c motion.Cast) *tintTable {
	var t tintTable
	for ch, gain := range c {
		for v := range 256 {
			t[ch][v] = to8(float32(float64(v) * gain))
		}
	}
	return &t
}

// copy writes the RGBA pixels of src, tinted, to dst. Alpha is kept as is.
func (t *tintTable) copy(dst, src []byte) {
	_ = dst[len(src)-1]
	for i := 0; i+3 < len(src); i += 4 {
		dst[i] = t[0][src[i]]
		dst[i+1] = t[1][src[i+1]]
		dst[i+2] = t[2][src[i+2]]
		dst[i+3] = src[i+3]
	}
}

// gains is the cast as multipliers for float drawing (see drawBilinear).
func gains(c motion.Cast) [3]float32 {
	return [3]float32{float32(c[0]), float32(c[1]), float32(c[2])}
}

// noTint leaves colors as they are.
var noTint = [3]float32{1, 1, 1}

// castPeriod is how many appearances one pass of a looping clip holds, for
// motion.Tinter.Cast: n frames of hold frames each, times perStep appearances
// per switch. 0 (casts never repeat) when the clip does not loop.
func castPeriod(loop bool, n, hold, perStep int) int {
	if !loop {
		return 0
	}
	return (n + hold - 1) / hold * perStep
}
