package ffmpeg

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	for _, want := range []string{"-loglevel error", "-i i.jpg", "-i a.wav", "-vf null", "-t 10.000", "-preset ultrafast"} {
		if !strings.Contains(args, want) {
			t.Errorf("args missing %q: %s", want, args)
		}
	}
	if !strings.HasSuffix(args, "o.mp4") {
		t.Errorf("output path should be last: %s", args)
	}
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
