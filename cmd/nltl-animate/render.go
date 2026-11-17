package main

import (
	"bufio"
	"errors"
	"fmt"
	"image"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nothinglefttolearn/nltl-animate/internal/config"
	"github.com/nothinglefttolearn/nltl-animate/internal/ffmpeg"
	"github.com/nothinglefttolearn/nltl-animate/internal/frames"
	"github.com/nothinglefttolearn/nltl-animate/internal/preset"
	"github.com/nothinglefttolearn/nltl-animate/internal/style"
	"github.com/nothinglefttolearn/nltl-animate/internal/tempo"
	"github.com/nothinglefttolearn/nltl-animate/internal/video"
)

type clipStyle string

const (
	styleSingle clipStyle = "single"
	styleGrid   clipStyle = "grid"
	styleTrain  clipStyle = "train"
)

// Grid tile modes when the grid has several images.
const (
	tilesSame  = "same"  // every tile shows the same image, switching together
	tilesMixed = "mixed" // every tile has its own image sequence
)

// defaultHoldFrames is how long each image shows when several images switch
// over time and neither --hold, --hold-beats nor --bpm is given (0.5 s).
const defaultHoldFrames = 15

// oneImageLoopSeconds is how long a silent loop of a single image lasts:
// long enough for the preset's wandering motion to read as movement.
const oneImageLoopSeconds = 3

// renderOptions is everything needed to render one clip. It is filled from
// flags, and in interactive mode the TUI overrides the fields it asks about.
type renderOptions struct {
	Style      clipStyle
	AudioPath  string   // empty with NoAudio
	NoAudio    bool     // silent clip that loops seamlessly (see loopFrames)
	Overwrite  bool     // replace an existing output file without asking
	ImagePaths []string // one or more
	PresetName string
	BPM        int
	AutoBPM    bool // detect BPM from the audio; BPM is filled in by renderClip
	Preview    bool
	Still      bool
	Verbose    bool
	OutputDir  string
	ConfigPath string

	// switching between several images (single and grid)
	Hold      int     // frames per image; 0 = default (see holdFrames)
	HoldBeats float64 // beats per image; 0 = default
	Seed      uint64  // 0 = pick one at render time

	// grid
	GridN int
	Tiles string // tilesSame or tilesMixed

	// train (Train.BPM and Train.BG are filled from BPM and TrainBG)
	Train   style.TrainOptions
	TrainBG string
}

func defaultRenderOptions() renderOptions {
	return renderOptions{
		Style:      styleSingle,
		PresetName: "industrial",
		GridN:      3,
		Tiles:      tilesSame,
		Train:      style.DefaultTrainOptions(),
		TrainBG:    "#000000",
	}
}

func run(opts renderOptions) error {
	cfg, err := loadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	return renderClip(&opts, cfg)
}

// loadConfig returns an empty config when path is empty, so callers never
// need a nil check.
func loadConfig(path string) (*config.Config, error) {
	if path == "" {
		return &config.Config{}, nil
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	return cfg, nil
}

// resolvePreset looks the name up in the config first, then the built-ins.
func resolvePreset(name string, cfg *config.Config) (preset.Preset, error) {
	if p, ok := cfg.GetPreset(name); ok {
		return p, nil
	}
	if p, ok := preset.Get(name); ok {
		return p, nil
	}
	if len(cfg.Presets) > 0 {
		return preset.Preset{}, fmt.Errorf("preset %q not found in config or built-ins — built-ins: %s",
			name, strings.Join(preset.Names(), ", "))
	}
	return preset.Preset{}, fmt.Errorf("unknown preset %q — try: %s", name, strings.Join(preset.Names(), ", "))
}

// validate checks the options that do not need any files. It fills
// Train.BPM and Train.BG as a side effect.
func (o *renderOptions) validate() error {
	if o.BPM < 0 {
		return fmt.Errorf("--bpm must be positive (or 0 for time-based animation), got %d", o.BPM)
	}
	if o.NoAudio && o.AutoBPM {
		return errors.New("--bpm auto needs an audio track — pass --bpm <number> with --no-audio")
	}

	if len(o.ImagePaths) == 0 {
		return fmt.Errorf("%s style needs at least 1 image", o.Style)
	}
	if o.Hold < 0 {
		return fmt.Errorf("--hold must be at least 1 frame, got %d", o.Hold)
	}
	if o.HoldBeats < 0 {
		return fmt.Errorf("--hold-beats must be positive, got %g", o.HoldBeats)
	}
	if o.HoldBeats > 0 && o.BPM == 0 && !o.AutoBPM {
		return errors.New("--hold-beats needs --bpm")
	}
	if o.HoldBeats > 0 && o.Hold > 0 {
		return errors.New("use either --hold or --hold-beats, not both")
	}

	switch o.Style {
	case styleSingle:
	case styleGrid:
		if o.GridN < 1 || o.GridN > style.MaxGrid {
			return fmt.Errorf("--n must be between 1 and %d, got %d", style.MaxGrid, o.GridN)
		}
		switch o.Tiles {
		case tilesSame:
		case tilesMixed:
			if len(o.ImagePaths) < 2 {
				return errors.New("--tiles mixed needs at least 2 images")
			}
		default:
			return fmt.Errorf("--tiles must be %q or %q, got %q", tilesSame, tilesMixed, o.Tiles)
		}
	case styleTrain:
		bg, err := style.ParseColor(o.TrainBG)
		if err != nil {
			return fmt.Errorf("--bg: %w", err)
		}
		o.Train.BG, o.Train.BPM = bg, o.BPM
		if err := o.Train.Validate(); err != nil {
			return fmt.Errorf("train: %w", err)
		}
	default:
		return fmt.Errorf("unknown style %q", o.Style)
	}
	return nil
}

// switches reports whether the clip switches between images over time:
// single and grid with several images. (Train shows several images at once
// instead.)
func (o *renderOptions) switches() bool {
	return len(o.ImagePaths) > 1 && o.Style != styleTrain
}

// holdFrames is how many frames each image shows: --hold-beats, then
// --hold, then one beat when the BPM is known, else defaultHoldFrames.
func (o *renderOptions) holdFrames() int {
	switch {
	case o.HoldBeats > 0:
		return style.HoldFromBeats(o.HoldBeats, o.BPM)
	case o.Hold > 0:
		return o.Hold
	case o.BPM > 0:
		return style.HoldFromBeats(1, o.BPM)
	default:
		return defaultHoldFrames
	}
}

// outputPrefix starts every output file name.
const outputPrefix = "nltl_animate"

// timestampLayout is the render time in output file names, so repeated
// renders never collide.
const timestampLayout = "20060102-150405"

// now is the clock for output file names; tests replace it.
var now = time.Now

// outputPath is nltl_animate_{style}[_detail]_{audio|loop_noaudio}[_preview]_{timestamp}.mp4
// next to the audio file, or the first image with --no-audio (or in
// --output-dir).
func (o *renderOptions) outputPath(at time.Time) string {
	dir := o.OutputDir
	if dir == "" {
		near := o.AudioPath
		if o.NoAudio {
			near = o.ImagePaths[0]
		}
		dir = filepath.Dir(near)
	}

	var suffix string
	switch o.Style {
	case styleSingle:
		suffix = "single"
		if o.switches() {
			suffix = "cycle"
		}
	case styleGrid:
		suffix = "grid" + strconv.Itoa(o.GridN)
		if o.switches() && o.Tiles == tilesMixed {
			suffix += "_mixed"
		}
	default:
		suffix = string(o.Style)
	}
	if o.NoAudio {
		suffix += "_loop_noaudio"
	} else {
		suffix += "_audio"
	}
	if o.Preview {
		suffix += "_preview"
	}
	return filepath.Join(dir, fmt.Sprintf("%s_%s_%s.mp4", outputPrefix, suffix, at.Format(timestampLayout)))
}

// Overridable in tests: where the overwrite question is read from, and
// whether that is a person at a terminal.
var (
	promptIn      io.Reader = os.Stdin
	promptIsHuman           = func() bool {
		fi, err := os.Stdin.Stat()
		return err == nil && fi.Mode()&os.ModeCharDevice != 0
	}
)

// confirmOverwrite returns nil if path does not exist, if --overwrite was
// given, or if the user agrees to replace it. Without a terminal to ask on,
// an existing file is an error.
func (o *renderOptions) confirmOverwrite(path string) error {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("checking output file: %w", err)
	}
	if o.Overwrite {
		return nil
	}
	if !promptIsHuman() {
		return fmt.Errorf("%s already exists — pass --overwrite to replace it", path)
	}
	fmt.Printf("%s %s already exists. Overwrite? [y/N] ", warnStyle.Render("Warning:"), path)
	answer, _ := bufio.NewReader(promptIn).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	}
	return errors.New("not overwriting " + path)
}

// loopFrames is the length of a silent clip, chosen so it loops seamlessly:
// one pass through the image sequence (each image once, or for train one
// image cycle of the cars), or oneImageLoopSeconds with a single image. The
// preset's wobble is not matched up at the loop point; it is jumpy anyway.
func (o *renderOptions) loopFrames() int {
	oneImage := oneImageLoopSeconds * video.FPS
	switch {
	case o.Style == styleTrain && len(o.ImagePaths) > 1:
		return style.TrainLoopFrames(o.Train, len(o.ImagePaths), 0)
	case o.Style == styleTrain:
		return style.TrainLoopFrames(o.Train, 1, oneImage)
	case o.switches():
		return len(o.ImagePaths) * o.holdFrames()
	default:
		return oneImage
	}
}

// videoInput returns the ffmpeg video input (still image or frame source)
// and filtergraph for the style.
func (o *renderOptions) videoInput(p preset.Preset) (ffmpeg.Options, error) {
	w, h := video.Size, video.Size
	switch {
	case o.Style == styleGrid && !o.switches():
		return ffmpeg.Options{ImagePath: o.ImagePaths[0], Filter: style.GridFilter(p, w, h, o.GridN, o.BPM)}, nil

	case o.Style == styleGrid && o.Tiles == tilesMixed:
		tw, th := style.GridTileSize(w, h, o.GridN)
		imgs, err := loadCovered(o.ImagePaths, tw, th)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		src, err := style.NewMosaic(imgs, o.GridN, o.holdFrames(), o.Seed)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		src.Loop = o.NoAudio
		return ffmpeg.Options{Frames: src, Filter: style.MosaicFilter(p, w, h, o.GridN, o.BPM)}, nil

	case o.Style == styleGrid: // same image in every tile, switching together
		tw, th := style.GridTileSize(w, h, o.GridN)
		imgs, err := loadCovered(o.ImagePaths, tw, th)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		src, err := style.NewCycle(imgs, o.holdFrames(), o.Seed)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		src.Loop = o.NoAudio
		return ffmpeg.Options{Frames: src, Filter: style.GridFilter(p, w, h, o.GridN, o.BPM)}, nil

	case o.Style == styleSingle && o.switches():
		imgs, err := loadCovered(o.ImagePaths, w, h)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		src, err := style.NewCycle(imgs, o.holdFrames(), o.Seed)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		src.Loop = o.NoAudio
		return ffmpeg.Options{Frames: src, Filter: p.BuildFilter(w, h, o.BPM)}, nil

	case o.Style == styleTrain:
		// Convert each image as it is decoded so only one decoder-format
		// copy is alive at a time.
		imgs := make([]image.Image, 0, len(o.ImagePaths))
		for _, path := range o.ImagePaths {
			img, err := frames.Load(path)
			if err != nil {
				return ffmpeg.Options{}, err
			}
			imgs = append(imgs, frames.ToRGBA(img))
		}
		train := o.Train
		if o.NoAudio {
			train.LoopFrames = o.loopFrames()
		}
		src, err := style.NewTrain(imgs, w, h, train)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		return ffmpeg.Options{Frames: src, Filter: p.BuildFilter(w, h, o.BPM)}, nil

	default:
		return ffmpeg.Options{ImagePath: o.ImagePaths[0], Filter: p.BuildFilter(w, h, o.BPM)}, nil
	}
}

// loadCovered decodes each image and cover-crops it to w x h, one at a time
// so large originals never all sit in memory together.
func loadCovered(paths []string, w, h int) ([]*image.RGBA, error) {
	imgs := make([]*image.RGBA, 0, len(paths))
	for _, path := range paths {
		img, err := frames.Load(path)
		if err != nil {
			return nil, err
		}
		imgs = append(imgs, frames.Cover(img, w, h))
	}
	return imgs, nil
}

// styleSummary describes the style settings for the pre-render summary.
func (o *renderOptions) styleSummary() string {
	switch o.Style {
	case styleSingle:
		if o.switches() {
			return fmt.Sprintf("single — %d images, new image %s (seed %d)", len(o.ImagePaths), o.holdSummary(), o.Seed)
		}
	case styleGrid:
		grid := fmt.Sprintf("grid — %d×%d", o.GridN, o.GridN)
		switch {
		case !o.switches():
			return grid
		case o.Tiles == tilesMixed:
			return fmt.Sprintf("%s, %d images mixed across the tiles, new images %s (seed %d)",
				grid, len(o.ImagePaths), o.holdSummary(), o.Seed)
		default:
			return fmt.Sprintf("%s, %d images, same in every tile, new image %s (seed %d)",
				grid, len(o.ImagePaths), o.holdSummary(), o.Seed)
		}
	case styleTrain:
		rate := fmtFloat(o.Train.Speed) + " per second"
		switch {
		case o.BPM > 0 && o.Train.Beats == 1:
			rate = "one per beat"
		case o.BPM > 0:
			rate = fmt.Sprintf("one every %s beats", fmtFloat(o.Train.Beats))
		}
		if n := len(o.ImagePaths); n > 1 {
			return fmt.Sprintf("train — %d copies cycling %d images, %s", o.Train.Copies, n, rate)
		}
		return fmt.Sprintf("train — %d copies, %s", o.Train.Copies, rate)
	}
	return ""
}

// holdSummary describes holdFrames for the summary, e.g. "every beat".
func (o *renderOptions) holdSummary() string {
	switch {
	case o.HoldBeats == 1, o.HoldBeats == 0 && o.Hold == 0 && o.BPM > 0:
		return "every beat"
	case o.HoldBeats > 0:
		return fmt.Sprintf("every %s beats", fmtFloat(o.HoldBeats))
	case o.Hold == 1:
		return "every frame"
	default:
		return fmt.Sprintf("every %d frames", o.holdFrames())
	}
}

// headlessCommand returns the flag-driven invocation equivalent to o. Style
// flags are included only when they differ from the defaults; the cycle seed
// is always included so the clip can be reproduced.
func (o *renderOptions) headlessCommand() string {
	d := defaultRenderOptions()
	parts := []string{"nltl-animate"}
	if o.Style != styleSingle {
		parts = append(parts, string(o.Style))
	}
	parts = append(parts, "--preset", o.PresetName)
	if o.BPM > 0 {
		parts = append(parts, "--bpm", strconv.Itoa(o.BPM))
	}
	if o.Still {
		parts = append(parts, "--still")
	}
	if o.Preview {
		parts = append(parts, "--preview")
	}
	if o.NoAudio {
		parts = append(parts, "--no-audio")
	}
	if o.ConfigPath != "" {
		parts = append(parts, "--config", o.ConfigPath)
	}
	if o.OutputDir != "" {
		parts = append(parts, "--output-dir", o.OutputDir)
	}

	flagIf := func(changed bool, name, value string) {
		if changed {
			parts = append(parts, name, value)
		}
	}
	if o.switches() {
		flagIf(o.Hold != d.Hold, "--hold", strconv.Itoa(o.Hold))
		flagIf(o.HoldBeats != d.HoldBeats, "--hold-beats", fmtFloat(o.HoldBeats))
		flagIf(o.Seed != 0, "--seed", strconv.FormatUint(o.Seed, 10))
	}
	switch o.Style {
	case styleGrid:
		flagIf(true, "--n", strconv.Itoa(o.GridN))
		flagIf(o.switches() && o.Tiles != d.Tiles, "--tiles", o.Tiles)
	case styleTrain:
		t, dt := o.Train, d.Train
		flagIf(t.Copies != dt.Copies, "--copies", strconv.Itoa(t.Copies))
		flagIf(t.Spacing != dt.Spacing, "--spacing", fmtFloat(t.Spacing))
		flagIf(t.Speed != dt.Speed, "--speed", fmtFloat(t.Speed))
		flagIf(t.Beats != dt.Beats, "--beats", fmtFloat(t.Beats))
		flagIf(t.Offset != dt.Offset, "--offset", fmtFloat(t.Offset))
		flagIf(t.FOV != dt.FOV, "--fov", fmtFloat(t.FOV))
		flagIf(t.Vanish != dt.Vanish, "--vanish", fmtFloat(t.Vanish))
		flagIf(t.Fog != dt.Fog, "--fog", fmtFloat(t.Fog))
		flagIf(o.TrainBG != d.TrainBG, "--bg", o.TrainBG)
	}

	if !o.NoAudio {
		parts = append(parts, o.AudioPath)
	}
	parts = append(parts, o.ImagePaths...)
	return strings.Join(parts, " ")
}

func fmtFloat(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

// maxTempoSeconds caps how much of the track is decoded for BPM detection.
const maxTempoSeconds = 300

func detectBPM(audioPath string) (int, error) {
	samples, err := ffmpeg.DecodeMono(audioPath, tempo.SampleRate, maxTempoSeconds)
	if err != nil {
		return 0, fmt.Errorf("decoding audio for BPM detection: %w", err)
	}
	bpm, err := tempo.Detect(samples)
	if err != nil {
		return 0, fmt.Errorf("detecting BPM: %w — pass --bpm <number> instead", err)
	}
	return bpm, nil
}

func renderClip(o *renderOptions, cfg *config.Config) error {
	if err := o.validate(); err != nil {
		return err
	}
	outPath := o.outputPath(now())
	if err := o.confirmOverwrite(outPath); err != nil {
		return err
	}
	if o.AutoBPM {
		bpm, err := detectBPM(o.AudioPath)
		if err != nil {
			return err
		}
		o.BPM, o.Train.BPM = bpm, bpm
	}
	if o.switches() && o.Seed == 0 {
		o.Seed = rand.Uint64() | 1 //nolint:gosec // visual randomness; |1 keeps it non-zero
	}

	p, err := resolvePreset(o.PresetName, cfg)
	if err != nil {
		return err
	}
	if o.Still {
		p.ZoomAmp = 0
		p.JitterAmp = 0
		p.ZoomBase = 1.0
	}

	var duration float64
	if !o.NoAudio {
		duration, err = ffmpeg.GetDuration(o.AudioPath)
		if err != nil {
			return fmt.Errorf("reading audio duration: %w", err)
		}
	}

	ff, err := o.videoInput(p)
	if err != nil {
		return err
	}
	ff.AudioPath = o.AudioPath
	ff.OutputPath = outPath
	ff.Duration = duration
	if o.NoAudio {
		ff.NumFrames = o.loopFrames()
	}
	ff.Preview = o.Preview
	ff.Verbose = o.Verbose

	fmt.Println()
	if s := o.styleSummary(); s != "" {
		fmt.Println(label("Style") + valueStyle.Render(s))
	}
	fmt.Println(label("Preset") + valueStyle.Render(fmt.Sprintf("%s — %s", p.Name, p.Description)))
	if o.BPM > 0 {
		bpm := strconv.Itoa(o.BPM)
		if o.AutoBPM {
			bpm += labelStyle.Render(" (detected — pass --bpm to override)")
		}
		fmt.Println(label("BPM") + valueStyle.Render(bpm))
	}
	if o.NoAudio {
		fmt.Println(label("Loop") + valueStyle.Render(fmt.Sprintf("%ss (%d frames), no audio",
			strconv.FormatFloat(float64(ff.NumFrames)/video.FPS, 'f', 2, 64), ff.NumFrames)))
	}
	if o.Preview {
		fmt.Println(label("Preview") + valueStyle.Render(fmt.Sprintf("%ds low-quality render", video.PreviewSeconds)))
	}
	if o.Still {
		fmt.Println(label("Still") + valueStyle.Render("yes (spatial motion disabled)"))
	}
	fmt.Println(label("Output") + accentStyle.Render(ff.OutputPath))
	fmt.Println()

	return ffmpeg.Render(ff)
}
