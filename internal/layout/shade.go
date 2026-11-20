package layout

// shadeTable is one shade as a lookup, so applying it while copying an image
// costs three table lookups per pixel and no stored copy.
type shadeTable [256]uint8

func newShadeTable(gain float64) *shadeTable {
	var t shadeTable
	for v := range 256 {
		t[v] = to8(float32(float64(v) * gain))
	}
	return &t
}

// copy writes the RGBA pixels of src, shaded, to dst. Alpha is kept as is.
func (t *shadeTable) copy(dst, src []byte) {
	_ = dst[len(src)-1]
	for i := 0; i+3 < len(src); i += 4 {
		dst[i] = t[src[i]]
		dst[i+1] = t[src[i+1]]
		dst[i+2] = t[src[i+2]]
		dst[i+3] = src[i+3]
	}
}

// noShade leaves brightness as it is.
const noShade float32 = 1

// shadePeriod is how many appearances one pass of a looping clip holds, for
// motion.Shader.Gain: n frames of hold frames each, times perStep
// appearances per switch. 0 (shades never repeat) when the clip does not
// loop.
func shadePeriod(loop bool, n, hold, perStep int) int {
	if !loop {
		return 0
	}
	return (n + hold - 1) / hold * perStep
}
