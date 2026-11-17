package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
		{"needs an image", func(o *renderOptions) { o.ImagePaths = nil }, "at least 1 image"},
		{"single several images", func(o *renderOptions) { o.ImagePaths = []string{"a", "b"} }, ""},
		{"negative hold", func(o *renderOptions) { o.ImagePaths, o.Hold = []string{"a", "b"}, -1 }, "--hold"},
		{"hold-beats with auto bpm", func(o *renderOptions) {
			o.ImagePaths, o.HoldBeats, o.AutoBPM = []string{"a", "b"}, 1, true
		}, ""},
		{"hold-beats needs bpm", func(o *renderOptions) {
			o.ImagePaths, o.HoldBeats = []string{"a", "b"}, 1
		}, "needs --bpm"},
		{"hold and hold-beats", func(o *renderOptions) {
			o.ImagePaths, o.HoldBeats, o.Hold, o.BPM = []string{"a", "b"}, 1, 4, 120
		}, "either --hold or --hold-beats"},
		{"grid range", func(o *renderOptions) { o.Style, o.GridN = styleGrid, 13 }, "--n"},
		{"grid several images", func(o *renderOptions) { o.Style, o.ImagePaths = styleGrid, []string{"a", "b"} }, ""},
		{"grid mixed", func(o *renderOptions) {
			o.Style, o.ImagePaths, o.Tiles = styleGrid, []string{"a", "b"}, tilesMixed
		}, ""},
		{"grid mixed needs two", func(o *renderOptions) { o.Style, o.Tiles = styleGrid, tilesMixed }, "at least 2 images"},
		{"grid bad tiles", func(o *renderOptions) { o.Style, o.Tiles = styleGrid, "random" }, "--tiles"},
		{"train bad bg", func(o *renderOptions) { o.Style, o.TrainBG = styleTrain, "red" }, "--bg"},
		{"train bad option", func(o *renderOptions) { o.Style, o.Train.Fog = styleTrain, 3 }, "fog"},
		{"no audio", func(o *renderOptions) { o.NoAudio, o.AudioPath, o.BPM = true, "", 120 }, ""},
		{"no audio auto bpm", func(o *renderOptions) { o.NoAudio, o.AudioPath, o.AutoBPM = true, "", true }, "needs an audio track"},
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
	cases := []struct {
		o    func() renderOptions
		want string
	}{
		{func() renderOptions { return opts(styleSingle, "i") }, "track_single.mp4"},
		{func() renderOptions { o := opts(styleGrid, "i"); o.GridN = 5; return o }, "track_grid5.mp4"},
		{func() renderOptions { return opts(styleSingle, "a", "b") }, "track_cycle.mp4"},
		{func() renderOptions { return opts(styleGrid, "a", "b") }, "track_grid3.mp4"},
		{func() renderOptions { o := opts(styleGrid, "a", "b"); o.Tiles = tilesMixed; return o }, "track_grid3_mixed.mp4"},
		{func() renderOptions { o := opts(styleTrain, "i"); o.Preview = true; return o }, "track_train_preview.mp4"},
	}
	for _, c := range cases {
		o := c.o()
		if got := o.outputPath(); got != c.want {
			t.Errorf("%s: got %q, want %q", o.Style, got, c.want)
		}
	}
	o := opts(styleGrid, "/art/a.png", "/art/b.png")
	o.NoAudio, o.AudioPath, o.Tiles = true, "", tilesMixed
	if got := o.outputPath(); got != filepath.Join("/art", "a_grid3_mixed_loop.mp4") {
		t.Errorf("no audio: got %q", got)
	}

	o = opts(styleSingle, "i")
	o.AudioPath = "/music/song.mp4"
	if got := o.outputPath(); got == o.AudioPath {
		t.Errorf("output %q would overwrite the audio", got)
	}
	o.AudioPath, o.OutputDir = "/music/song.flac", "/out"
	if got := o.outputPath(); got != filepath.Join("/out", "song_single.mp4") {
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
// When ffmpeg is asked to decode audio to raw PCM it prints the file named by
// $FAKE_PCM, or fails if that is unset.
func fakeFFmpeg(t *testing.T) (argsFile, stdinFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake tools are shell scripts")
	}
	dir := t.TempDir()
	argsFile, stdinFile = filepath.Join(dir, "args"), filepath.Join(dir, "stdin")
	tools := map[string]string{
		"ffprobe": "echo 1.0\n",
		"ffmpeg": `case "$*" in *f32le*) [ -n "$FAKE_PCM" ] && exec cat "$FAKE_PCM"; echo "Invalid data found" >&2; exit 1 ;; esac` + "\n" +
			`printf '%s\n' "$@" > ` + argsFile + "\n" +
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
		t.Fatalf("nltl-animate %v: %v", args, err)
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
	frameBytes := "139968000" // 30 frames * 1080*1080*4 bytes (1s)

	t.Run("single", func(t *testing.T) {
		args, stdin := runCLI(t, audio, red)
		if argAfter(args, "-i") != red || stdin != "" {
			t.Errorf("single should loop the still image: %v", args)
		}
		if out := args[len(args)-1]; out != filepath.Join(dir, "song_single.mp4") {
			t.Errorf("output = %s", out)
		}
	})

	t.Run("grid", func(t *testing.T) {
		args, _ := runCLI(t, "grid", "--n", "4", audio, red)
		fc := argAfter(args, "-filter_complex")
		if !strings.Contains(fc, "xstack=inputs=16:grid=4x4") || !strings.Contains(fc, "scale=1080:1080") {
			t.Errorf("filter = %s", fc)
		}
		if out := args[len(args)-1]; !strings.HasSuffix(out, "song_grid4.mp4") {
			t.Errorf("output = %s", out)
		}
	})

	t.Run("single with several images", func(t *testing.T) {
		args, stdin := runCLI(t, "--seed", "7", audio, red, blue)
		if argAfter(args, "-i") != "pipe:0" || argAfter(args, "-video_size") != "1080x1080" {
			t.Errorf("several images should stream frames: %v", args)
		}
		if stdin != frameBytes {
			t.Errorf("ffmpeg read %s bytes, want %s", stdin, frameBytes)
		}
		if out := args[len(args)-1]; !strings.HasSuffix(out, "song_cycle.mp4") {
			t.Errorf("output = %s", out)
		}
	})

	t.Run("grid same images", func(t *testing.T) {
		args, stdin := runCLI(t, "grid", "--n", "2", "--hold", "5", audio, red, blue)
		// Frames are one tile (540x540); the filter tiles them.
		if argAfter(args, "-video_size") != "540x540" || stdin != "34992000" { // 30 * 540*540*4
			t.Errorf("grid same should stream tile-size frames: %v (stdin %s)", args, stdin)
		}
		if fc := argAfter(args, "-filter_complex"); !strings.Contains(fc, "split=4") || strings.Contains(fc, "crop=540:540:540") {
			t.Errorf("grid same should split one moving tile: %s", fc)
		}
	})

	t.Run("grid mixed images", func(t *testing.T) {
		args, stdin := runCLI(t, "grid", "--n", "2", "--tiles", "mixed", audio, red, blue)
		if argAfter(args, "-video_size") != "1080x1080" || stdin != frameBytes {
			t.Errorf("grid mixed should stream whole-grid frames: %v (stdin %s)", args, stdin)
		}
		fc := argAfter(args, "-filter_complex")
		if strings.Count(fc, "zoompan") != 4 || !strings.Contains(fc, "crop=540:540:540:540") {
			t.Errorf("grid mixed should move each tile separately: %s", fc)
		}
		if out := args[len(args)-1]; !strings.HasSuffix(out, "song_grid2_mixed.mp4") {
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

	t.Run("train with several images", func(t *testing.T) {
		args, stdin := runCLI(t, "train", "--copies", "5", audio, red, blue)
		if argAfter(args, "-i") != "pipe:0" || stdin != frameBytes {
			t.Errorf("train should stream frames: %v (stdin %s)", args, stdin)
		}
		if out := args[len(args)-1]; !strings.HasSuffix(out, "song_train.mp4") {
			t.Errorf("output = %s", out)
		}
	})
}

// writeKickPCM writes seconds of mono float32 PCM at 11025 Hz with a kick
// drum on every beat at bpm, in the format ffmpeg.DecodeMono reads.
func writeKickPCM(t *testing.T, dir string, bpm, seconds float64) string {
	t.Helper()
	const rate = 11025
	n := int(seconds * rate)
	buf := make([]byte, 4*n)
	beat := 60 / bpm
	for i := range n {
		sec := float64(i) / rate
		since := math.Mod(sec, beat)
		v := 0.9 * math.Sin(2*math.Pi*55*since) * math.Exp(-since*15)
		binary.LittleEndian.PutUint32(buf[4*i:], math.Float32bits(float32(v)))
	}
	path := filepath.Join(dir, "kick.f32")
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLIAutoBPM(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.wav")
	red := writePNG(t, dir, "red.png", color.RGBA{R: 255, A: 255})
	t.Setenv("FAKE_PCM", writeKickPCM(t, dir, 128, 20))

	auto, _ := runCLI(t, "--bpm", "auto", audio, red)
	manual, _ := runCLI(t, "--bpm", "128", audio, red)
	none, _ := runCLI(t, audio, red)
	if argAfter(auto, "-filter_complex") == argAfter(none, "-filter_complex") {
		t.Fatal("BPM does not change the filter, so this test proves nothing")
	}
	if a, m := argAfter(auto, "-filter_complex"), argAfter(manual, "-filter_complex"); a != m {
		t.Errorf("--bpm auto should render as --bpm 128\nauto:   %s\nmanual: %s", a, m)
	}
}

func TestCLIAutoBPMFailure(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.wav")
	red := writePNG(t, dir, "red.png", color.RGBA{R: 255, A: 255})
	fakeFFmpeg(t) // FAKE_PCM unset: decoding fails

	cmd := newRootCmd()
	cmd.SetArgs([]string{"--bpm", "auto", audio, red})
	cmd.SetOut(new(strings.Builder))
	cmd.SetErr(new(strings.Builder))
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "BPM detection") || !strings.Contains(err.Error(), "Invalid data found") {
		t.Errorf("err = %v", err)
	}
}

func TestBPMFlag(t *testing.T) {
	var o renderOptions
	f := bpmFlag{&o}
	if err := f.Set("auto"); err != nil || !o.AutoBPM || f.String() != "auto" {
		t.Errorf("auto: %v %+v", err, o)
	}
	if err := f.Set("140"); err != nil || o.AutoBPM || o.BPM != 140 || f.String() != "140" {
		t.Errorf("140: %v %+v", err, o)
	}
	if err := f.Set("fast"); err == nil {
		t.Error("expected an error for a non-number")
	}
}

func TestCLIArgErrors(t *testing.T) {
	for _, args := range [][]string{
		{"cycle", "a.wav", "one.png", "two.png"}, // removed style: explained, not run
		{"grid", "a.wav"},                        // missing image
		{"a.wav"},
	} {
		cmd := newRootCmd()
		cmd.SetArgs(args)
		cmd.SetOut(new(strings.Builder))
		cmd.SetErr(new(strings.Builder))
		if err := cmd.Execute(); err == nil {
			t.Errorf("nltl-animate %v: expected an argument error", args)
		}
	}
}

func TestLoopFrames(t *testing.T) {
	cases := []struct {
		name string
		mod  func(*renderOptions)
		want int
	}{
		{"one image", func(o *renderOptions) {}, 90},
		{"grid one image", func(o *renderOptions) { o.Style = styleGrid }, 90},
		{"several images", func(o *renderOptions) { o.ImagePaths = []string{"a", "b", "c"} }, 3 * defaultHoldFrames},
		{"several images at a bpm", func(o *renderOptions) { o.ImagePaths, o.BPM = []string{"a", "b"}, 120 }, 2 * 15},
		{"grid mixed", func(o *renderOptions) {
			o.Style, o.ImagePaths, o.Tiles, o.Hold = styleGrid, []string{"a", "b", "c", "d"}, tilesMixed, 2
		}, 4 * 2},
		{"train one image", func(o *renderOptions) { o.Style = styleTrain }, 100},
		{"train several images", func(o *renderOptions) { o.Style, o.ImagePaths = styleTrain, []string{"a", "b"} }, 40},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := opts(styleSingle, "i.png")
			o.NoAudio, o.AudioPath = true, ""
			c.mod(&o)
			if err := o.validate(); err != nil {
				t.Fatal(err)
			}
			if got := o.loopFrames(); got != c.want {
				t.Errorf("loopFrames = %d, want %d", got, c.want)
			}
		})
	}
}

func TestCLINoAudio(t *testing.T) {
	dir := t.TempDir()
	red := writePNG(t, dir, "red.png", color.RGBA{R: 255, A: 255})
	blue := writePNG(t, dir, "blue.png", color.RGBA{B: 255, A: 255})
	green := writePNG(t, dir, "green.png", color.RGBA{G: 255, A: 255})
	silent := func(t *testing.T, args []string, frames string) {
		t.Helper()
		if got := argAfter(args, "-frames:v"); got != frames {
			t.Errorf("-frames:v = %q, want %s: %v", got, frames, args)
		}
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "1:a") || strings.Contains(joined, "-c:a") {
			t.Errorf("silent clip maps audio: %v", args)
		}
	}

	t.Run("one image", func(t *testing.T) {
		args, _ := runCLI(t, "--no-audio", red)
		silent(t, args, "90")
		if argAfter(args, "-i") != red {
			t.Errorf("single should loop the still image: %v", args)
		}
		if out := args[len(args)-1]; out != filepath.Join(dir, "red_single_loop.mp4") {
			t.Errorf("output = %s", out)
		}
	})

	t.Run("several images", func(t *testing.T) {
		args, stdin := runCLI(t, "--no-audio", "--hold", "4", "--seed", "3", red, blue, green)
		silent(t, args, "12")
		if stdin != "55987200" { // 12 frames * 1080*1080*4
			t.Errorf("ffmpeg read %s bytes, want 12 frames", stdin)
		}
		if out := args[len(args)-1]; out != filepath.Join(dir, "red_cycle_loop.mp4") {
			t.Errorf("output = %s", out)
		}
	})

	t.Run("grid mixed", func(t *testing.T) {
		args, _ := runCLI(t, "grid", "--no-audio", "--n", "2", "--tiles", "mixed", "--bpm", "120", red, blue, green)
		silent(t, args, "45") // 3 images, a beat (15 frames) each
	})

	t.Run("train", func(t *testing.T) {
		args, _ := runCLI(t, "train", "--no-audio", "--copies", "4", red, blue)
		silent(t, args, "40") // two copies at 1.5 per second
	})
}

func TestCLINoAudioArgErrors(t *testing.T) {
	for _, args := range [][]string{
		{"grid", "--no-audio"},
		{"--no-audio", "--bpm", "auto", "a.png"},
	} {
		cmd := newRootCmd()
		cmd.SetArgs(args)
		cmd.SetOut(new(strings.Builder))
		cmd.SetErr(new(strings.Builder))
		if err := cmd.Execute(); err == nil {
			t.Errorf("nltl-animate %v: expected an error", args)
		}
	}
}

func TestHeadlessCommandNoAudio(t *testing.T) {
	o := opts(styleSingle, "a.png", "b.png")
	o.NoAudio, o.AudioPath, o.Seed = true, "", 5
	if got, want := o.headlessCommand(), "nltl-animate --preset industrial --no-audio --seed 5 a.png b.png"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}
