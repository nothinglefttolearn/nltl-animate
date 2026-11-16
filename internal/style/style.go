// Package style builds the video side of each clip style: either an ffmpeg
// filtergraph over a looped still image, or a Go frame renderer whose raw
// frames are piped to ffmpeg (see ffmpeg.FrameSource).
package style

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// ParseColor parses "#rgb" or "#rrggbb" (the leading # is optional).
func ParseColor(s string) (color.RGBA, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return color.RGBA{}, fmt.Errorf("invalid colour %q: want #rgb or #rrggbb", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid colour %q: want #rgb or #rrggbb", s)
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, nil //nolint:gosec // masked by uint8
}
