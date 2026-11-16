// Package video holds constants shared by the filter builders, the frame
// renderers and the ffmpeg runner, so they cannot drift apart.
package video

const (
	// FPS is the output frame rate. Filter expressions that count frames
	// (zoompan's "on") and the raw frame streams both assume it.
	FPS = 30

	// PreviewSeconds is the maximum length of a --preview render.
	PreviewSeconds = 10
)

// FrameCount is the number of frames needed to cover seconds of audio.
func FrameCount(seconds float64) int {
	n := int(seconds * FPS)
	if float64(n) < seconds*FPS {
		n++
	}
	return n
}
