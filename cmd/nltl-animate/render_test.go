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
	"time"

	"github.com/nothinglefttolearn/nltl-animate/internal/outdir"
)

// TestMain keeps the developer's own output directory out of the tests; a
// test that needs it sets it with t.Setenv.
func TestMain(m *testing.M) {
	_ = os.Unsetenv(outdir.EnvVar)
	os.Exit(m.Run())
}

func opts(l layoutName, images ...string) renderOptions {
	o := defaultRenderOptions()
	o.Layout, o.AudioPath, o.ImagePaths = l, "track.wav", images
	return o
}

func TestValidateDetectsBPMWithAudio(t *testing.T) {
	o := opts(layoutSingle, "i.png")
	if err := o.validate(); err != nil || !o.AutoBPM {
		t.Errorf("audio without --bpm: AutoBPM = %v, err = %v; want detection", o.AutoBPM, err)
	}
	o = opts(layoutSingle, "i.png")
	o.BPM = 140
	if err := o.validate(); err != nil || o.AutoBPM {
		t.Errorf("--bpm 140: AutoBPM = %v, err = %v; want no detection", o.AutoBPM, err)
	}
	o = opts(layoutSingle, "i.png")
	o.NoAudio, o.AudioPath = true, ""
	if err := o.validate(); err != nil || o.AutoBPM {
		t.Errorf("--no-audio: AutoBPM = %v, err = %v; want no detection", o.AutoBPM, err)
	}
	o = opts(layoutSingle, "i.png")
	o.NoBPM = true
	if err := o.validate(); err != nil || o.AutoBPM {
		t.Errorf("--bpm disable: AutoBPM = %v, err = %v; want no detection", o.AutoBPM, err)
	}
	o = opts(layoutSingle, "a.png", "b.png")
	o.NoBPM, o.HoldBeats = true, 2
	if err := o.validate(); err == nil || !strings.Contains(err.Error(), "--hold-beats needs a tempo") {
		t.Errorf("--bpm disable --hold-beats 2: err = %v, want a tempo error", err)
	}
	o.HoldBeats, o.Hold = 0, 10
	if err := o.validate(); err != nil {
		t.Errorf("--bpm disable --hold 10: %v", err)
	}
}

func TestHeadlessCommandDisabledBPM(t *testing.T) {
	o := opts(layoutSingle, "a.png")
	o.NoBPM = true
	if got, want := o.headlessCommand(), "nltl-animate --grade muted --motion steady --bpm disable track.wav a.png"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	o.NoAudio, o.AudioPath = true, ""
	if got := o.headlessCommand(); strings.Contains(got, "--bpm") {
		t.Errorf("a silent loop has no tempo anyway; got %s", got)
	}
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
		{"hold-beats with detected bpm", func(o *renderOptions) {
			o.ImagePaths, o.HoldBeats = []string{"a", "b"}, 1
		}, ""},
		{"hold-beats needs bpm without audio", func(o *renderOptions) {
			o.NoAudio, o.AudioPath, o.ImagePaths, o.HoldBeats = true, "", []string{"a", "b"}, 1
		}, "needs --bpm"},
		{"hold and hold-beats", func(o *renderOptions) {
			o.ImagePaths, o.HoldBeats, o.Hold, o.BPM = []string{"a", "b"}, 1, 4, 120
		}, "either --hold or --hold-beats"},
		{"grid range", func(o *renderOptions) { o.Layout, o.GridN = layoutGrid, 13 }, "--n"},
		{"grid several images", func(o *renderOptions) { o.Layout, o.ImagePaths = layoutGrid, []string{"a", "b"} }, ""},
		{"grid mixed", func(o *renderOptions) {
			o.Layout, o.ImagePaths, o.Tiles = layoutGrid, []string{"a", "b"}, tilesMixed
		}, ""},
		{"grid mixed needs two", func(o *renderOptions) { o.Layout, o.Tiles = layoutGrid, tilesMixed }, "at least 2 images"},
		{"grid bad tiles", func(o *renderOptions) { o.Layout, o.Tiles = layoutGrid, "random" }, "--tiles"},
		{"train bad bg", func(o *renderOptions) { o.Layout, o.TrainBG = layoutTrain, "red" }, "--bg"},
		{"train bad option", func(o *renderOptions) { o.Layout, o.Train.Fog = layoutTrain, 3 }, "fog"},
		{"no audio", func(o *renderOptions) { o.NoAudio, o.AudioPath, o.BPM = true, "", 120 }, ""},
		{"no audio without bpm", func(o *renderOptions) { o.NoAudio, o.AudioPath = true, "" }, ""},
		{"no audio auto bpm", func(o *renderOptions) { o.NoAudio, o.AudioPath, o.AutoBPM = true, "", true }, "needs an audio track"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := opts(layoutSingle, "i.png")
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
	o := opts(layoutTrain, "i.png")
	o.BPM, o.TrainBG = 128, "#ff0000"
	if err := o.validate(); err != nil {
		t.Fatal(err)
	}
	if o.Train.BPM != 128 || o.Train.BG != (color.RGBA{R: 255, A: 255}) {
		t.Errorf("train options not filled: %+v", o.Train)
	}
}

// testTime is the clock in tests, so output names are known: stamp is its
// file-name form.
var testTime = time.Date(2026, 11, 17, 21, 53, 1, 0, time.UTC)

const stamp = "20261117-215301"

func TestOutputPath(t *testing.T) {
	cases := []struct {
		o    func() renderOptions
		want string
	}{
		{func() renderOptions { return opts(layoutSingle, "i") }, "nltl_animate_single_audio_" + stamp + ".mp4"},
		{func() renderOptions { o := opts(layoutGrid, "i"); o.GridN = 5; return o }, "nltl_animate_grid5_audio_" + stamp + ".mp4"},
		{func() renderOptions { return opts(layoutSingle, "a", "b") }, "nltl_animate_cycle_audio_" + stamp + ".mp4"},
		{func() renderOptions { return opts(layoutGrid, "a", "b") }, "nltl_animate_grid3_audio_" + stamp + ".mp4"},
		{func() renderOptions { o := opts(layoutGrid, "a", "b"); o.Tiles = tilesMixed; return o }, "nltl_animate_grid3_mixed_audio_" + stamp + ".mp4"},
		{func() renderOptions { o := opts(layoutTrain, "i"); o.Preview = true; return o }, "nltl_animate_train_audio_preview_" + stamp + ".mp4"},
	}
	for _, c := range cases {
		o := c.o()
		if got := o.outputPath(testTime); got != c.want {
			t.Errorf("%s: got %q, want %q", o.Layout, got, c.want)
		}
	}
	o := opts(layoutGrid, "/art/a.png", "/art/b.png")
	o.NoAudio, o.AudioPath, o.Tiles = true, "", tilesMixed
	if got := o.outputPath(testTime); got != filepath.Join("/art", "nltl_animate_grid3_mixed_loop_noaudio_"+stamp+".mp4") {
		t.Errorf("no audio: got %q", got)
	}
	o = opts(layoutSingle, "i")
	o.AudioPath, o.OutputDir = "/music/song.flac", "/out"
	if got := o.outputPath(testTime); got != filepath.Join("/out", "nltl_animate_single_audio_"+stamp+".mp4") {
		t.Errorf("got %q", got)
	}
	if a, b := o.outputPath(testTime), o.outputPath(testTime.Add(time.Second)); a == b {
		t.Errorf("renders a second apart share a name: %s", a)
	}
}

// fakePrompt makes the overwrite question come from a person typing answer,
// or from no terminal when human is false.
func fakePrompt(t *testing.T, human bool, answer string) {
	t.Helper()
	oldIn, oldHuman := promptIn, promptIsHuman
	promptIn, promptIsHuman = strings.NewReader(answer), func() bool { return human }
	t.Cleanup(func() { promptIn, promptIsHuman = oldIn, oldHuman })
}

func TestConfirmOverwrite(t *testing.T) {
	existing := filepath.Join(t.TempDir(), "out.mp4")
	if err := os.WriteFile(existing, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name      string
		path      string
		overwrite bool
		human     bool
		answer    string
		wantErr   string
	}{
		{"new file", existing + ".new", false, false, "", ""},
		{"--overwrite", existing, true, false, "", ""},
		{"no terminal", existing, false, false, "", "--overwrite"},
		{"yes", existing, false, true, "y\n", ""},
		{"YES", existing, false, true, " YES \n", ""},
		{"no", existing, false, true, "n\n", "not overwriting"},
		{"just enter", existing, false, true, "\n", "not overwriting"},
		{"end of input", existing, false, true, "", "not overwriting"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fakePrompt(t, c.human, c.answer)
			o := opts(layoutSingle, "i")
			o.Overwrite = c.overwrite
			err := o.confirmOverwrite(c.path)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v, want containing %q", err, c.wantErr)
			}
		})
	}
}

func TestCLIRefusesToOverwriteWithoutAsking(t *testing.T) {
	dir := t.TempDir()
	red := writePNG(t, dir, "red.png", color.RGBA{R: 255, A: 255})
	useTestTime(t)
	fakePrompt(t, false, "")
	fakeFFmpeg(t)
	out := filepath.Join(dir, "nltl_animate_single_loop_noaudio_"+stamp+".mp4")
	if err := os.WriteFile(out, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCmd()
	cmd.SetArgs([]string{"--no-audio", red})
	cmd.SetOut(new(strings.Builder))
	cmd.SetErr(new(strings.Builder))
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("err = %v", err)
	}
	args, _ := runCLI(t, "--no-audio", "--overwrite", red)
	if got := args[len(args)-1]; got != out {
		t.Errorf("--overwrite rendered to %s, want %s", got, out)
	}
}

func useTestTime(t *testing.T) {
	t.Helper()
	old := now
	now = func() time.Time { return testTime }
	t.Cleanup(func() { now = old })
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
// $FAKE_PCM (a kick on every beat at 120 BPM if FAKE_PCM is not set), or
// fails if FAKE_PCM is set but empty.
func fakeFFmpeg(t *testing.T) (argsFile, stdinFile string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake tools are shell scripts")
	}
	dir := t.TempDir()
	if _, ok := os.LookupEnv("FAKE_PCM"); !ok {
		t.Setenv("FAKE_PCM", writeKickPCM(t, dir, 120, 20))
	}
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
	useTestTime(t)
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

func TestCLILayouts(t *testing.T) {
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
		if out := args[len(args)-1]; out != filepath.Join(dir, "nltl_animate_single_audio_"+stamp+".mp4") {
			t.Errorf("output = %s", out)
		}
	})

	t.Run("grid", func(t *testing.T) {
		args, _ := runCLI(t, "grid", "--n", "4", audio, red)
		fc := argAfter(args, "-filter_complex")
		if !strings.Contains(fc, "xstack=inputs=16:grid=4x4") || !strings.Contains(fc, "scale=1080:1080") {
			t.Errorf("filter = %s", fc)
		}
		if out := args[len(args)-1]; !strings.HasSuffix(out, "nltl_animate_grid4_audio_"+stamp+".mp4") {
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
		if out := args[len(args)-1]; !strings.HasSuffix(out, "nltl_animate_cycle_audio_"+stamp+".mp4") {
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
		if out := args[len(args)-1]; !strings.HasSuffix(out, "nltl_animate_grid2_mixed_audio_"+stamp+".mp4") {
			t.Errorf("output = %s", out)
		}
	})

	t.Run("train", func(t *testing.T) {
		args, stdin := runCLI(t, "train", "--bpm", "120", "--bg", "#202020", "--copies", "8", audio, red)
		if argAfter(args, "-i") != "pipe:0" || stdin != frameBytes {
			t.Errorf("train should stream frames: %v (stdin %s)", args, stdin)
		}
		if fc := argAfter(args, "-filter_complex"); !strings.Contains(fc, "zoompan") || !strings.Contains(fc, "noise=") {
			t.Errorf("train should get the full motion and grade: %s", fc)
		}
	})

	t.Run("train with several images", func(t *testing.T) {
		args, stdin := runCLI(t, "train", "--copies", "5", audio, red, blue)
		if argAfter(args, "-i") != "pipe:0" || stdin != frameBytes {
			t.Errorf("train should stream frames: %v (stdin %s)", args, stdin)
		}
		if out := args[len(args)-1]; !strings.HasSuffix(out, "nltl_animate_train_audio_"+stamp+".mp4") {
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
	other, _ := runCLI(t, "--bpm", "120", audio, red)
	if argAfter(manual, "-filter_complex") == argAfter(other, "-filter_complex") {
		t.Fatal("BPM does not change the filter, so this test proves nothing")
	}
	m := argAfter(manual, "-filter_complex")
	if a := argAfter(auto, "-filter_complex"); a != m {
		t.Errorf("--bpm auto should render as --bpm 128\nauto:   %s\nmanual: %s", a, m)
	}
	if n := argAfter(none, "-filter_complex"); n != m {
		t.Errorf("no --bpm should detect the tempo and render as --bpm 128\nnone:   %s\nmanual: %s", n, m)
	}
}

func TestCLIDisabledBPM(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.wav")
	red := writePNG(t, dir, "red.png", color.RGBA{R: 255, A: 255})
	t.Setenv("FAKE_PCM", "") // detection would fail: disable must not try

	disabled, _ := runCLI(t, "--bpm", "disable", audio, red)
	assumed, _ := runCLI(t, "--bpm", "120", audio, red)
	if d, a := argAfter(disabled, "-filter_complex"), argAfter(assumed, "-filter_complex"); d != a {
		t.Errorf("--bpm disable should move at the assumed 120 BPM\ndisabled: %s\n120:      %s", d, a)
	}
}

func TestCLIAutoBPMFailure(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.wav")
	red := writePNG(t, dir, "red.png", color.RGBA{R: 255, A: 255})
	t.Setenv("FAKE_PCM", "") // decoding fails
	fakeFFmpeg(t)

	for _, args := range [][]string{{"--bpm", "auto", audio, red}, {audio, red}} {
		cmd := newRootCmd()
		cmd.SetArgs(args)
		cmd.SetOut(new(strings.Builder))
		cmd.SetErr(new(strings.Builder))
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "BPM detection") || !strings.Contains(err.Error(), "Invalid data found") {
			t.Errorf("%v: err = %v, want a detection error rather than an assumed tempo", args, err)
		}
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
	if err := f.Set("disable"); err != nil || o.AutoBPM || !o.NoBPM || o.BPM != 0 || f.String() != "disable" {
		t.Errorf("disable: %v %+v", err, o)
	}
	if err := f.Set("auto"); err != nil || o.NoBPM {
		t.Errorf("auto after disable: %v %+v", err, o)
	}
	if err := f.Set("fast"); err == nil {
		t.Error("expected an error for a non-number")
	}
}

func TestCLIArgErrors(t *testing.T) {
	for _, args := range [][]string{
		{"cycle", "a.wav", "one.png", "two.png"}, // removed command: explained, not run
		{"grid", "a.wav"},                        // missing image
		{"a.wav"},
		{"--preset", "dark", "a.wav", "i.png"}, // removed: --grade and --motion
		{"--still", "a.wav", "i.png"},          // removed: --motion still
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

func TestCLIGradeAndMotion(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.wav")
	red := writePNG(t, dir, "red.png", color.RGBA{R: 255, A: 255})

	args, _ := runCLI(t, "--grade", "dark", "--motion", "still", audio, red)
	fc := argAfter(args, "-filter_complex")
	if !strings.Contains(fc, "noise=alls=40") || !strings.Contains(fc, "saturation=0.40") {
		t.Errorf("dark grade not applied: %s", fc)
	}
	if !strings.Contains(fc, "z='1.0000+0.0000*") || !strings.Contains(fc, "brightness='0.0000*") {
		t.Errorf("still motion should not zoom or pulse: %s", fc)
	}

	cfg := filepath.Join(dir, "nltl.yaml")
	if err := os.WriteFile(cfg, []byte("grades:\n  - name: mine\n    grain: 77\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args, _ = runCLI(t, "--config", cfg, "--grade", "mine", audio, red)
	if fc := argAfter(args, "-filter_complex"); !strings.Contains(fc, "noise=alls=77") {
		t.Errorf("custom grade not applied: %s", fc)
	}

	for _, bad := range [][]string{{"--grade", "steady"}, {"--motion", "muted"}} {
		cmd := newRootCmd()
		cmd.SetArgs(append(bad, audio, red))
		cmd.SetOut(new(strings.Builder))
		cmd.SetErr(new(strings.Builder))
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unknown") {
			t.Errorf("%v: err = %v, want unknown grade/motion", bad, err)
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
		{"grid one image", func(o *renderOptions) { o.Layout = layoutGrid }, 90},
		{"several images", func(o *renderOptions) { o.ImagePaths = []string{"a", "b", "c"} }, 3 * defaultHoldFrames},
		{"several images at a bpm", func(o *renderOptions) { o.ImagePaths, o.BPM = []string{"a", "b"}, 120 }, 2 * 15},
		{"grid mixed", func(o *renderOptions) {
			o.Layout, o.ImagePaths, o.Tiles, o.Hold = layoutGrid, []string{"a", "b", "c", "d"}, tilesMixed, 2
		}, 4 * 2},
		{"train one image", func(o *renderOptions) { o.Layout = layoutTrain }, 100},
		{"train several images", func(o *renderOptions) { o.Layout, o.ImagePaths = layoutTrain, []string{"a", "b"} }, 40},
		// A shaded single image cycles through shades: whole switches, at
		// least oneImageLoopSeconds.
		{"one image shaded", func(o *renderOptions) { o.Shaded = true }, 6 * defaultHoldFrames},
		{"one image shaded at a bpm", func(o *renderOptions) { o.Shaded, o.BPM = true, 138 }, 7 * 13},
		{"grid one image shaded", func(o *renderOptions) { o.Layout, o.Shaded = layoutGrid, true }, 90},
		{"several images shaded", func(o *renderOptions) { o.ImagePaths, o.Shaded = []string{"a", "b"}, true }, 2 * defaultHoldFrames},
		{"train shaded", func(o *renderOptions) { o.Layout, o.Shaded = layoutTrain, true }, 100},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := opts(layoutSingle, "i.png")
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
		if out := args[len(args)-1]; out != filepath.Join(dir, "nltl_animate_single_loop_noaudio_"+stamp+".mp4") {
			t.Errorf("output = %s", out)
		}
	})

	t.Run("several images", func(t *testing.T) {
		args, stdin := runCLI(t, "--no-audio", "--hold", "4", "--seed", "3", red, blue, green)
		silent(t, args, "12")
		if stdin != "55987200" { // 12 frames * 1080*1080*4
			t.Errorf("ffmpeg read %s bytes, want 12 frames", stdin)
		}
		if out := args[len(args)-1]; out != filepath.Join(dir, "nltl_animate_cycle_loop_noaudio_"+stamp+".mp4") {
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

func TestCLIShade(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.wav")
	red := writePNG(t, dir, "red.png", color.RGBA{R: 255, A: 255})
	blue := writePNG(t, dir, "blue.png", color.RGBA{B: 255, A: 255})
	frameBytes := "139968000" // 30 frames * 1080*1080*4 bytes (1s)

	t.Run("single image", func(t *testing.T) {
		args, stdin := runCLI(t, "--motion", "shifting", audio, red)
		if argAfter(args, "-i") != "pipe:0" || stdin != frameBytes {
			t.Errorf("a shaded single image should stream shaded copies: %v (stdin %s)", args, stdin)
		}
		if out := args[len(args)-1]; !strings.HasSuffix(out, "nltl_animate_single_audio_"+stamp+".mp4") {
			t.Errorf("output = %s", out)
		}
	})

	for name, extra := range map[string][]string{
		"grid one image":   {red},
		"grid same images": {red, blue},
	} {
		t.Run(name, func(t *testing.T) {
			args, stdin := runCLI(t, append([]string{"grid", "--n", "2", "--motion", "shifting", audio}, extra...)...)
			// Each tile has its own shade, so frames are the whole grid
			// and every tile moves separately.
			if argAfter(args, "-video_size") != "1080x1080" || stdin != frameBytes {
				t.Errorf("shaded grid should stream whole-grid frames: %v (stdin %s)", args, stdin)
			}
			if fc := argAfter(args, "-filter_complex"); strings.Count(fc, "zoompan") != 4 {
				t.Errorf("shaded grid should move each tile separately: %s", fc)
			}
		})
	}

	t.Run("no shade keeps the still image", func(t *testing.T) {
		args, _ := runCLI(t, "grid", "--n", "2", "--motion", "steady", audio, red)
		if argAfter(args, "-i") != red {
			t.Errorf("an unshaded grid of one image should loop the still image: %v", args)
		}
	})

	t.Run("no audio", func(t *testing.T) {
		args, _ := runCLI(t, "--no-audio", "--motion", "shifting", "--bpm", "120", red)
		if got := argAfter(args, "-frames:v"); got != "90" { // 6 beats of 15 frames
			t.Errorf("-frames:v = %s, want 90", got)
		}
	})
}

func TestHeadlessCommandShade(t *testing.T) {
	o := opts(layoutSingle, "a.png")
	o.MotionName, o.Shaded, o.Seed, o.HoldBeats, o.BPM = "shifting", true, 5, 2, 120
	if got, want := o.headlessCommand(), "nltl-animate --grade muted --motion shifting --bpm 120 --hold-beats 2 --seed 5 track.wav a.png"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	o = opts(layoutTrain, "a.png")
	o.MotionName, o.Shaded, o.Seed = "shifting", true, 6
	if got, want := o.headlessCommand(), "nltl-animate train --grade muted --motion shifting --seed 6 track.wav a.png"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestTrainWithSeveralImagesIsSeeded(t *testing.T) {
	o := opts(layoutTrain, "a.png")
	if o.seeded() {
		t.Error("a train of one image without a shade should not use the seed")
	}
	o = opts(layoutTrain, "a.png", "b.png", "c.png")
	o.Seed, o.BPM = 9, 120
	if !o.seeded() {
		t.Fatal("a train of several images should use the seed for their order")
	}
	if got, want := o.headlessCommand(), "nltl-animate train --grade muted --motion steady --bpm 120 --seed 9 track.wav a.png b.png c.png"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
	if got, want := o.layoutSummary(), "train — 24 copies of 3 images in random order, one per beat (seed 9)"; got != want {
		t.Errorf("summary %q, want %q", got, want)
	}
}

func TestHeadlessCommandNoAudio(t *testing.T) {
	o := opts(layoutSingle, "a.png", "b.png")
	o.NoAudio, o.AudioPath, o.Seed = true, "", 5
	if got, want := o.headlessCommand(), "nltl-animate --grade muted --motion steady --no-audio --seed 5 a.png b.png"; got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestCLIOutputDir(t *testing.T) {
	dir := t.TempDir()
	audio := filepath.Join(dir, "song.wav")
	red := writePNG(t, dir, "red.png", color.RGBA{R: 255, A: 255})
	envDir, flagDir := t.TempDir(), t.TempDir()
	out := func(args []string) string { return filepath.Dir(args[len(args)-1]) }

	t.Run("variable", func(t *testing.T) {
		t.Setenv(outdir.EnvVar, envDir)
		if got := out(must2(runCLI(t, audio, red))); got != envDir {
			t.Errorf("output dir = %s, want %s", got, envDir)
		}
	})
	t.Run("flag beats variable", func(t *testing.T) {
		t.Setenv(outdir.EnvVar, envDir)
		if got := out(must2(runCLI(t, "-o", flagDir, audio, red))); got != flagDir {
			t.Errorf("output dir = %s, want %s", got, flagDir)
		}
	})
	t.Run("empty variable is unset", func(t *testing.T) {
		t.Setenv(outdir.EnvVar, "")
		if got := out(must2(runCLI(t, audio, red))); got != dir {
			t.Errorf("output dir = %s, want next to the audio %s", got, dir)
		}
	})

	for name, c := range map[string]struct {
		env  string
		args []string
		want string
	}{
		"missing from variable": {filepath.Join(dir, "gone"), nil, "does not exist (from " + outdir.EnvVar + ")"},
		"missing from flag":     {"", []string{"-o", filepath.Join(dir, "gone")}, "does not exist (from --output-dir)"},
		"file from variable":    {red, nil, "is a file, not a directory (from " + outdir.EnvVar + ")"},
	} {
		t.Run(name, func(t *testing.T) {
			argsFile, _ := fakeFFmpeg(t)
			t.Setenv(outdir.EnvVar, c.env)
			cmd := newRootCmd()
			cmd.SetArgs(append(c.args, audio, red))
			cmd.SetOut(new(strings.Builder))
			cmd.SetErr(new(strings.Builder))
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want containing %q", err, c.want)
			}
			if _, err := os.Stat(argsFile); err == nil {
				t.Error("ffmpeg ran despite a bad output directory")
			}
		})
	}
}

func must2(args []string, _ string) []string { return args }

func TestHeadlessCommandOutputDirFromVariable(t *testing.T) {
	o := opts(layoutSingle, "a.png")
	o.OutputDir, o.outputDirFrom = "/videos", outdir.EnvVar
	if got := o.headlessCommand(); strings.Contains(got, "--output-dir") {
		t.Errorf("a directory from %s should not become a flag: %s", outdir.EnvVar, got)
	}
}
