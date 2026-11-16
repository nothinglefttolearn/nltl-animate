package video

import "testing"

func TestFrameCount(t *testing.T) {
	for in, want := range map[float64]int{0: 0, 1: 30, 1.01: 31, 10: 300, 0.0334: 2} {
		if got := FrameCount(in); got != want {
			t.Errorf("FrameCount(%v) = %d, want %d", in, got, want)
		}
	}
}
