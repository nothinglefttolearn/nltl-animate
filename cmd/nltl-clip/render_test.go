package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nothinglefttolearn/nltl-clip/internal/profile"
)

func opts(s clipStyle, images ...string) renderOptions {
	o := defaultRenderOptions()
	o.Style, o.AudioPath, o.ImagePaths = s, "track.wav", images
	return o
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mod     func(*renderOptions)
		wantErr string
	}{
		{"single ok", func(o *renderOptions) {}, ""},
		{"negative bpm", func(o *renderOptions) { o.BPM = -1 }, "--bpm"},
		{"single needs one image", func(o *renderOptions) { o.ImagePaths = []string{"a", "b"} }, "exactly 1 image"},
		{"cycle needs two", func(o *renderOptions) { o.Style = styleCycle }, "at least 2 images"},
		{"cycle ok", func(o *renderOptions) { o.Style, o.ImagePaths = styleCycle, []string{"a", "b"} }, ""},
		{"cycle hold", func(o *renderOptions) { o.Style, o.ImagePaths, o.Hold = styleCycle, []string{"a", "b"}, 0 }, "--hold"},
		{"hold-beats needs bpm", func(o *renderOptions) {
			o.Style, o.ImagePaths, o.HoldBeats = styleCycle, []string{"a", "b"}, 1
		}, "needs --bpm"},
		{"hold and hold-beats", func(o *renderOptions) {
			o.Style, o.ImagePaths, o.HoldBeats, o.Hold, o.BPM = styleCycle, []string{"a", "b"}, 1, 4, 120
		}, "either --hold or --hold-beats"},
		{"grid range", func(o *renderOptions) { o.Style, o.GridN = styleGrid, 13 }, "--n"},
		{"train bad bg", func(o *renderOptions) { o.Style, o.TrainBG = styleTrain, "red" }, "--bg"},
		{"train bad option", func(o *renderOptions) { o.Style, o.Train.Fog = styleTrain, 3 }, "fog"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := opts(styleSingle, "i.png")
			tt.mod(&o)
			err := o.validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateFillsTrainFromFlags(t *testing.T) {
	o := opts(styleTrain, "i.png")
	o.BPM, o.TrainBG = 128, "#ff0000"
	if err := o.validate(); err != nil {
		t.Fatal(err)
	}
	if o.Train.BPM != 128 || o.Train.BG != (color.RGBA{R: 255, A: 255}) {
		t.Errorf("train options not filled: %+v", o.Train)
	}
}

func TestOutputPath(t *testing.T) {
	prof, _ := profile.Get("instagram-story")
	cases := []struct {
		o    func() renderOptions
		want string
	}{
		{func() renderOptions { return opts(styleSingle, "i") }, "track_instagram_story.mp4"},
		{func() renderOptions { o := opts(styleGrid, "i"); o.GridN = 5; return o }, "track_instagram_story_grid5.mp4"},
		{func() renderOptions { return opts(styleCycle, "a", "b") }, "track_instagram_story_cycle.mp4"},
		{func() renderOptions { o := opts(styleTrain, "i"); o.Preview = true; return o }, "track_instagram_story_train_preview.mp4"},
	}
	for _, c := range cases {
		o := c.o()
		if got := o.outputPath(prof); got != c.want {
			t.Errorf("%s: got %q, want %q", o.Style, got, c.want)
		}
	}
	o := opts(styleSingle, "i")
	o.AudioPath, o.OutputDir = "/music/song.flac", "/out"
	if got := o.outputPath(prof); got != filepath.Join("/out", "song_instagram_story.mp4") {
		t.Errorf("got %q", got)
	}
}

// --- end-to-end through the cobra commands, against fake ffprobe/ffmpeg ---

func writePNG(t *testing.T, dir, name string, c color.RGBA) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// fakeFFmpeg installs ffprobe (reports a 1s duration) and ffmpeg (records
// its args and the number of stdin bytes) and returns the files they write.
func fakeFFmpeg(t *testing.T) (argsFile, stdinFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake tools are shell scripts")
	}
	dir := t.TempDir()
	argsFile, stdinFile = filepath.Join(dir, "args"), filepath.Join(dir, "stdin")
	tools := map[string]string{
		"ffprobe": "echo 1.0\n",
		"ffmpeg": `printf '%s\n' "$@" > ` + argsFile + "\n" +
			`case "$*" in *pipe:0*) wc -c > ` + stdinFile + ` ;; esac` + "\n",
	}
	for name, script := range tools {
		//nolint:gosec // must be executable
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return argsFile, stdinFile
}

func runCLI(t *testing.T, args ...string) (ffArgs []string, stdinBytes string) {
	t.Helper()
	argsFile, stdinFile := fakeFFmpeg(t)
	cmd := newRootCmd()
	cmd.SetArgs(args)
	cmd.SetOut(new(strings.Builder))
	cmd.SetErr(new(strings.Builder))
	if err := cmd.Execute(); err != nil {
		t.Fatalf("nltl-clip %v: %v", args, err)
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("ffmpeg was not run: %v", err)
	}
	b, _ := os.ReadFile(stdinFile)
	return strings.Split(strings.TrimSpace(string(raw)), "\n"), strings.TrimSpace(string(b))
}

func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestCLIStyles(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.wav")
	red := writePNG(t, dir, "red.png", color.RGBA{R: 255, A: 255})
	blue := writePNG(t, dir, "blue.png", color.RGBA{B: 255, A: 255})
	frameBytes := "139968000" // 30 frames * 1080*1080*4 bytes (instagram-square, 1s)

	t.Run("single", func(t *testing.T) {
		args, stdin := runCLI(t, audio, red)
		if argAfter(args, "-i") != red || stdin != "" {
			t.Errorf("single should loop the still image: %v", args)
		}
		if out := args[len(args)-1]; out != filepath.Join(dir, "song_instagram_square.mp4") {
			t.Errorf("output = %s", out)
		}
	})

	t.Run("grid", func(t *testing.T) {
		args, _ := runCLI(t, "grid", "--n", "4", "--profile", "youtube", audio, red)
		fc := argAfter(args, "-filter_complex")
		if !strings.Contains(fc, "xstack=inputs=16:grid=4x4") || !strings.Contains(fc, "scale=1920:1080") {
			t.Errorf("filter = %s", fc)
		}
		if out := args[len(args)-1]; !strings.HasSuffix(out, "song_youtube_grid4.mp4") {
			t.Errorf("output = %s", out)
		}
	})

	t.Run("cycle", func(t *testing.T) {
		args, stdin := runCLI(t, "cycle", "--seed", "7", audio, red, blue)
		if argAfter(args, "-i") != "pipe:0" || argAfter(args, "-video_size") != "1080x1080" {
			t.Errorf("cycle should stream frames: %v", args)
		}
		if stdin != frameBytes {
			t.Errorf("ffmpeg read %s bytes, want %s", stdin, frameBytes)
		}
		if out := args[len(args)-1]; !strings.HasSuffix(out, "song_instagram_square_cycle.mp4") {
			t.Errorf("output = %s", out)
		}
	})

	t.Run("train", func(t *testing.T) {
		args, stdin := runCLI(t, "train", "--bpm", "120", "--bg", "#202020", "--copies", "8", audio, red)
		if argAfter(args, "-i") != "pipe:0" || stdin != frameBytes {
			t.Errorf("train should stream frames: %v (stdin %s)", args, stdin)
		}
		if fc := argAfter(args, "-filter_complex"); !strings.Contains(fc, "zoompan") || !strings.Contains(fc, "noise=") {
			t.Errorf("train should get the full preset: %s", fc)
		}
	})
}

func TestCLIArgErrors(t *testing.T) {
	for _, args := range [][]string{
		{"cycle", "a.wav", "one.png"}, // cycle needs 2 images
		{"grid", "a.wav"},             // missing image
		{"train", "a.wav", "x.png", "y.png"},
		{"a.wav"},
	} {
		cmd := newRootCmd()
		cmd.SetArgs(args)
		cmd.SetOut(new(strings.Builder))
		cmd.SetErr(new(strings.Builder))
		if err := cmd.Execute(); err == nil {
			t.Errorf("nltl-clip %v: expected an argument error", args)
		}
	}
}
