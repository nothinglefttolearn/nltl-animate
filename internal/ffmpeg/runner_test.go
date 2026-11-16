package ffmpeg

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// fakeTool puts an executable shell script called name first on PATH.
func fakeTool(t *testing.T, name, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake tools are shell scripts")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil { //nolint:gosec // must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestGetDuration(t *testing.T) {
	fakeTool(t, "ffprobe", "echo 12.345\n")
	d, err := GetDuration("a.wav")
	if err != nil || d != 12.345 {
		t.Fatalf("GetDuration = %v, %v; want 12.345", d, err)
	}
}

func TestGetDurationErrors(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr []string
	}{
		{"includes stderr", "echo 'a.wav: Invalid data found when processing input' >&2\nexit 1\n",
			[]string{"ffprobe failed", "exit status 1", "Invalid data found"}},
		{"unparseable output", "echo N/A\n", []string{"could not read a duration", `"N/A"`}},
		{"zero duration", "echo 0\n", []string{"could not read a duration"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeTool(t, "ffprobe", tt.script)
			_, err := GetDuration("a.wav")
			assertErrContains(t, err, tt.wantErr...)
		})
	}
}

func TestRenderSuccessAndArgs(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args")
	fakeTool(t, "ffmpeg", `printf '%s\n' "$@" > `+argsFile+"\n")

	err := Render(Options{ImagePath: "i.jpg", AudioPath: "a.wav", OutputPath: "o.mp4", Filter: "null", Duration: 30, Preview: true})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Join(strings.Fields(string(raw)), " ")
	for _, want := range []string{"-loglevel error", "-i i.jpg", "-i a.wav", "-filter_complex [0:v]null[v]", "-map [v]", "-map 1:a", "-loop 1", "-t 10.000", "-preset ultrafast"} {
		if !strings.Contains(args, want) {
			t.Errorf("args missing %q: %s", want, args)
		}
	}
	if !strings.HasSuffix(args, "o.mp4") {
		t.Errorf("output path should be last: %s", args)
	}
}

// solidFrames is a FrameSource of identical frames.
type solidFrames struct {
	w, h int
	fail error
}

func (s solidFrames) Size() (int, int) { return s.w, s.h }

func (s solidFrames) WriteFrames(w io.Writer, n int) error {
	if s.fail != nil {
		return s.fail
	}
	frame := make([]byte, s.w*s.h*4)
	for range n {
		if _, err := w.Write(frame); err != nil {
			return err
		}
	}
	return nil
}

func TestRenderFramesPipesEveryFrame(t *testing.T) {
	dir := t.TempDir()
	argsFile, countFile := filepath.Join(dir, "args"), filepath.Join(dir, "count")
	fakeTool(t, "ffmpeg", `printf '%s\n' "$@" > `+argsFile+"\nwc -c > "+countFile+"\n")

	err := Render(Options{Frames: solidFrames{w: 8, h: 6}, AudioPath: "a.wav", OutputPath: "o.mp4", Filter: "null", Duration: 2})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	raw, _ := os.ReadFile(countFile)
	if got, want := strings.TrimSpace(string(raw)), strconv.Itoa(60*8*6*4); got != want {
		t.Errorf("ffmpeg read %s bytes, want %s (60 frames of 8x6 RGBA)", got, want)
	}
	rawArgs, _ := os.ReadFile(argsFile)
	args := strings.Join(strings.Fields(string(rawArgs)), " ")
	for _, want := range []string{"-f rawvideo -pixel_format rgba -video_size 8x6 -framerate 30 -i pipe:0", "-crf 18"} {
		if !strings.Contains(args, want) {
			t.Errorf("args missing %q: %s", want, args)
		}
	}
	if strings.Contains(args, "stillimage") || strings.Contains(args, "-loop") {
		t.Errorf("still-image options used for a frame stream: %s", args)
	}
}

func TestRenderFramesFfmpegErrorWins(t *testing.T) {
	// ffmpeg dies without reading stdin; the writer then hits a broken pipe.
	fakeTool(t, "ffmpeg", "echo 'Invalid frame size' >&2\nexit 1\n")
	err := Render(Options{Frames: solidFrames{w: 64, h: 64}, OutputPath: "o.mp4", Duration: 60})
	assertErrContains(t, err, "ffmpeg failed", "exit status 1", "Invalid frame size")
	if strings.Contains(err.Error(), "broken pipe") || strings.Contains(err.Error(), "writing frames") {
		t.Errorf("write error leaked into message: %v", err)
	}
}

func TestRenderFramesEarlySuccessfulExit(t *testing.T) {
	// ffmpeg reads a little and exits 0, as it does once -t is reached.
	fakeTool(t, "ffmpeg", "head -c 100 > /dev/null\nexit 0\n")
	if err := Render(Options{Frames: solidFrames{w: 64, h: 64}, OutputPath: "o.mp4", Duration: 60}); err != nil {
		t.Errorf("Render: %v", err)
	}
}

func TestRenderFramesSourceError(t *testing.T) {
	fakeTool(t, "ffmpeg", "cat > /dev/null\n")
	err := Render(Options{Frames: solidFrames{w: 8, h: 8, fail: errors.New("decode boom")}, OutputPath: "o.mp4", Duration: 1})
	assertErrContains(t, err, "writing frames", "decode boom")
}

func TestRenderFailureIncludesStderr(t *testing.T) {
	fakeTool(t, "ffmpeg", "echo '[Parsed_zoompan_2] Invalid argument' >&2\necho 'Error opening filters!' >&2\nexit 234\n")
	err := Render(Options{OutputPath: "o.mp4", Duration: 1})
	assertErrContains(t, err, "ffmpeg failed", "exit status 234", "  [Parsed_zoompan_2] Invalid argument", "  Error opening filters!")
}

func TestRenderNotFound(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := Render(Options{OutputPath: "o.mp4", Duration: 1})
	assertErrContains(t, err, "ffmpeg not found in PATH", "install ffmpeg")
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("error should wrap exec.ErrNotFound: %v", err)
	}
}

func TestLastLines(t *testing.T) {
	if got := lastLines("a\nb\nc", 2); got != "...\nb\nc" {
		t.Errorf("lastLines truncated = %q", got)
	}
	if got := lastLines("a\nb", 5); got != "a\nb" {
		t.Errorf("lastLines short = %q", got)
	}
	if got := lastLines("", 5); got != "" {
		t.Errorf("lastLines empty = %q", got)
	}
}

func assertErrContains(t *testing.T, err error, want ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, s := range want {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("error missing %q:\n%v", s, err)
		}
	}
}
