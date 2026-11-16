package style

import (
	"image/color"
	"testing"

	"github.com/nothinglefttolearn/nltl-clip/internal/ffmpeg"
)

var (
	_ ffmpeg.FrameSource = (*Cycle)(nil)
	_ ffmpeg.FrameSource = (*Train)(nil)
)

func TestParseColor(t *testing.T) {
	ok := map[string]color.RGBA{
		"#000000": {A: 255},
		"ff8000":  {R: 255, G: 128, A: 255},
		"#fff":    {R: 255, G: 255, B: 255, A: 255},
		"#0a0B0c": {R: 10, G: 11, B: 12, A: 255},
	}
	for in, want := range ok {
		got, err := ParseColor(in)
		if err != nil || got != want {
			t.Errorf("ParseColor(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "#12", "#1234567", "black", "#gg0000"} {
		if _, err := ParseColor(in); err == nil {
			t.Errorf("ParseColor(%q) should fail", in)
		}
	}
}
