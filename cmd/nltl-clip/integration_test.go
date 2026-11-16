//go:build integration

// Integration tests render real clips with the ffmpeg/ffprobe on PATH.
// Run with: mise run test:integration

package main

import (
	"image/color"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestIntegrationRenderEveryStyle(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "tone.wav")
	if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "sine=f=220:d=1",
		"-ac", "2", audio).CombinedOutput(); err != nil {
		t.Fatalf("making test audio: %v\n%s", err, out)
	}
	red := writePNG(t, dir, "red.png", color.RGBA{R: 255, A: 255})
	half := writePNG(t, dir, "half.png", color.RGBA{G: 128, A: 128}) // translucent
	blue := writePNG(t, dir, "blue.png", color.RGBA{B: 255, A: 255})

	tests := []struct {
		name string
		args []string
		out  string
		w, h int
	}{
		{"single", []string{audio, red}, "tone_instagram_square_preview.mp4", 1080, 1080},
		{"grid", []string{"grid", "--n", "3", "-p", "youtube", audio, red}, "tone_youtube_grid3_preview.mp4", 1920, 1080},
		{"cycle", []string{"cycle", "--seed", "1", "-p", "instagram-story", audio, red, blue}, "tone_instagram_story_cycle_preview.mp4", 1080, 1920},
		{"train", []string{"train", "--bpm", "140", audio, half}, "tone_instagram_square_train_preview.mp4", 1080, 1080},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetArgs(append([]string{"--preview", "-o", dir}, tt.args...))
			if err := cmd.Execute(); err != nil {
				t.Fatalf("render: %v", err)
			}

			out := filepath.Join(dir, tt.out)
			probe, err := exec.Command("ffprobe", "-v", "error",
				"-show_entries", "stream=codec_type,width,height:format=duration",
				"-of", "default=noprint_wrappers=1", out).Output()
			if err != nil {
				t.Fatalf("ffprobe %s: %v", out, err)
			}
			info := string(probe)
			for _, want := range []string{
				"codec_type=video", "codec_type=audio",
				"width=" + strconv.Itoa(tt.w), "height=" + strconv.Itoa(tt.h),
			} {
				if !strings.Contains(info, want) {
					t.Errorf("%s: missing %q in\n%s", tt.out, want, info)
				}
			}
			for _, line := range strings.Split(info, "\n") {
				if d, ok := strings.CutPrefix(line, "duration="); ok {
					if f, _ := strconv.ParseFloat(d, 64); f < 0.9 || f > 1.2 {
						t.Errorf("duration = %s, want about 1s", d)
					}
				}
			}
		})
	}
}
