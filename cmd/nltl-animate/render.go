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
	"github.com/nothinglefttolearn/nltl-animate/internal/grade"
	"github.com/nothinglefttolearn/nltl-animate/internal/layout"
	"github.com/nothinglefttolearn/nltl-animate/internal/motion"
	"github.com/nothinglefttolearn/nltl-animate/internal/outdir"
	"github.com/nothinglefttolearn/nltl-animate/internal/tempo"
	"github.com/nothinglefttolearn/nltl-animate/internal/video"
)

// layoutName is how the images are arranged in the frame; each one is a
// subcommand ("single" is the root command).
type layoutName string

const (
	layoutSingle layoutName = "single"
	layoutGrid   layoutName = "grid"
	layoutTrain  layoutName = "train"
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
// long enough for the motion's breath and shake to read as movement.
const oneImageLoopSeconds = 3

// renderOptions is everything needed to render one clip. It is filled from
// flags, and in interactive mode the TUI overrides the fields it asks about.
type renderOptions struct {
	Layout     layoutName
	AudioPath  string   // empty with NoAudio
	NoAudio    bool     // silent clip that loops seamlessly (see loopFrames)
	Overwrite  bool     // replace an existing output file without asking
	ImagePaths []string // one or more
	GradeName  string
	MotionName string
	BPM        int
	AutoBPM    bool // detect BPM from the audio; BPM is filled in by renderClip
	Preview    bool
	Verbose    bool
	OutputDir  string // --output-dir; renderClip fills it from outdir.EnvVar when empty
	ConfigPath string

	// outputDirFrom names where OutputDir came from (outdir.FlagSource or
	// outdir.EnvVar); renderClip fills it in.
	outputDirFrom string

	// switching between several images, or shades (single and grid)
	Hold      int     // frames per image; 0 = default (see holdFrames)
	HoldBeats float64 // beats per image; 0 = default
	Seed      uint64  // image order and shades; 0 = pick one at render time

	// Shaded is whether the motion has a shade; renderClip fills it in.
	Shaded bool

	// grid
	GridN int
	Tiles string // tilesSame or tilesMixed

	// train (Train.BPM and Train.BG are filled from BPM and TrainBG)
	Train   layout.TrainOptions
	TrainBG string
}

func defaultRenderOptions() renderOptions {
	return renderOptions{
		Layout:     layoutSingle,
		GradeName:  grade.Default,
		MotionName: motion.Default,
		GridN:      3,
		Tiles:      tilesSame,
		Train:      layout.DefaultTrainOptions(),
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

// resolveGrade looks the name up in the config first, then the built-ins.
func resolveGrade(name string, cfg *config.Config) (grade.Grade, error) {
	if g, ok := cfg.GetGrade(name); ok {
		return g, nil
	}
	if g, ok := grade.Get(name); ok {
		return g, nil
	}
	return grade.Grade{}, fmt.Errorf("unknown grade %q — built-ins: %s%s",
		name, strings.Join(grade.Names(), ", "), customHint(len(cfg.Grades)))
}

// resolveMotion looks the name up in the config first, then the built-ins.
func resolveMotion(name string, cfg *config.Config) (motion.Motion, error) {
	if m, ok := cfg.GetMotion(name); ok {
		return m, nil
	}
	if m, ok := motion.Get(name); ok {
		return m, nil
	}
	return motion.Motion{}, fmt.Errorf("unknown motion %q — built-ins: %s%s",
		name, strings.Join(motion.Names(), ", "), customHint(len(cfg.Motions)))
}

func customHint(custom int) string {
	if custom > 0 {
		return fmt.Sprintf(" (and %d custom from --config)", custom)
	}
	return ""
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
		return fmt.Errorf("%s layout needs at least 1 image", o.Layout)
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

	switch o.Layout {
	case layoutSingle:
	case layoutGrid:
		if o.GridN < 1 || o.GridN > layout.MaxGrid {
			return fmt.Errorf("--n must be between 1 and %d, got %d", layout.MaxGrid, o.GridN)
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
	case layoutTrain:
		bg, err := layout.ParseColor(o.TrainBG)
		if err != nil {
			return fmt.Errorf("--bg: %w", err)
		}
		o.Train.BG, o.Train.BPM = bg, o.BPM
		if err := o.Train.Validate(); err != nil {
			return fmt.Errorf("train: %w", err)
		}
	default:
		return fmt.Errorf("unknown layout %q", o.Layout)
	}
	return nil
}

// switches reports whether the clip switches between images over time:
// single and grid with several images. (Train shows several images at once
// instead.)
func (o *renderOptions) switches() bool {
	return len(o.ImagePaths) > 1 && o.Layout != layoutTrain
}

// steps reports whether the clip moves through new appearances every hold:
// switching images, or a shade giving even a single image a new shade on
// every switch. (Train has its own pace.)
func (o *renderOptions) steps() bool {
	return o.switches() || o.Shaded && o.Layout != layoutTrain
}

// seeded reports whether the clip uses the random seed: for the image order
// or the shades.
func (o *renderOptions) seeded() bool {
	return o.switches() || o.Shaded
}

// holdFrames is how many frames each image shows: --hold-beats, then
// --hold, then one beat when the BPM is known, else defaultHoldFrames.
func (o *renderOptions) holdFrames() int {
	switch {
	case o.HoldBeats > 0:
		return layout.HoldFromBeats(o.HoldBeats, o.BPM)
	case o.Hold > 0:
		return o.Hold
	case o.BPM > 0:
		return layout.HoldFromBeats(1, o.BPM)
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

// resolveOutputDir picks the output directory from --output-dir or
// outdir.EnvVar and checks it before anything is rendered. With neither,
// outputPath writes next to the input.
func (o *renderOptions) resolveOutputDir() error {
	dir, from := o.OutputDir, o.outputDirFrom
	if from == "" {
		dir, from = outdir.Resolve(o.OutputDir)
	}
	if dir == "" {
		return nil
	}
	checked, err := outdir.Check(dir)
	if err != nil {
		return fmt.Errorf("%w (from %s)", err, from)
	}
	o.OutputDir, o.outputDirFrom = checked, from
	return nil
}

// outputPath is nltl_animate_{layout}[_detail]_{audio|loop_noaudio}[_preview]_{timestamp}.mp4
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
	switch o.Layout {
	case layoutSingle:
		suffix = "single"
		if o.switches() {
			suffix = "cycle"
		}
	case layoutGrid:
		suffix = "grid" + strconv.Itoa(o.GridN)
		if o.switches() && o.Tiles == tilesMixed {
			suffix += "_mixed"
		}
	default:
		suffix = string(o.Layout)
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
// image cycle of the cars), or oneImageLoopSeconds with a single image
// (rounded up to whole switches when it is shaded). The shades repeat with
// the loop. The motion's breath and shake are not matched up at the loop
// point; they are jumpy anyway.
func (o *renderOptions) loopFrames() int {
	oneImage := oneImageLoopSeconds * video.FPS
	switch {
	case o.Layout == layoutTrain && len(o.ImagePaths) > 1:
		return layout.TrainLoopFrames(o.Train, len(o.ImagePaths), 0)
	case o.Layout == layoutTrain:
		return layout.TrainLoopFrames(o.Train, 1, oneImage)
	case o.switches():
		return len(o.ImagePaths) * o.holdFrames()
	case o.steps():
		hold := o.holdFrames()
		return max(2, (oneImage+hold-1)/hold) * hold
	default:
		return oneImage
	}
}

// videoInput returns the ffmpeg video input (still image or frame source)
// and filtergraph for the layout, with the motion and grade over it.
func (o *renderOptions) videoInput(m motion.Motion, g grade.Grade) (ffmpeg.Options, error) {
	w, h := video.Size, video.Size
	switch {
	case o.Layout == layoutGrid && !o.steps():
		return ffmpeg.Options{ImagePath: o.ImagePaths[0], Filter: layout.GridFilter(m, g, w, h, o.GridN, o.BPM)}, nil

	// Mixed tiles, or shaded tiles that each need their own shade.
	case o.Layout == layoutGrid && (o.Tiles == tilesMixed && o.switches() || o.Shaded):
		tw, th := layout.GridTileSize(w, h, o.GridN)
		imgs, err := loadCovered(o.ImagePaths, tw, th)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		newMosaic := layout.NewSameMosaic
		if o.Tiles == tilesMixed && o.switches() {
			newMosaic = layout.NewMosaic
		}
		src, err := newMosaic(imgs, o.GridN, o.holdFrames(), o.Seed)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		src.Loop, src.Shade = o.NoAudio, m.Shader(o.Seed)
		return ffmpeg.Options{Frames: src, Filter: layout.MosaicFilter(m, g, w, h, o.GridN, o.BPM)}, nil

	case o.Layout == layoutGrid: // same image in every tile, switching together
		tw, th := layout.GridTileSize(w, h, o.GridN)
		imgs, err := loadCovered(o.ImagePaths, tw, th)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		src, err := layout.NewCycle(imgs, o.holdFrames(), o.Seed)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		src.Loop = o.NoAudio
		return ffmpeg.Options{Frames: src, Filter: layout.GridFilter(m, g, w, h, o.GridN, o.BPM)}, nil

	case o.Layout == layoutSingle && o.steps():
		imgs, err := loadCovered(o.ImagePaths, w, h)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		src, err := layout.NewCycle(imgs, o.holdFrames(), o.Seed)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		src.Loop, src.Shade = o.NoAudio, m.Shader(o.Seed)
		return ffmpeg.Options{Frames: src, Filter: layout.Filter(m, g, w, h, o.BPM)}, nil

	case o.Layout == layoutTrain:
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
		train.Shade = m.Shader(o.Seed)
		if o.NoAudio {
			train.LoopFrames = o.loopFrames()
		}
		src, err := layout.NewTrain(imgs, w, h, train)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		return ffmpeg.Options{Frames: src, Filter: layout.Filter(m, g, w, h, o.BPM)}, nil

	default:
		return ffmpeg.Options{ImagePath: o.ImagePaths[0], Filter: layout.Filter(m, g, w, h, o.BPM)}, nil
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

// layoutSummary describes the layout settings for the pre-render summary.
func (o *renderOptions) layoutSummary() string {
	switch o.Layout {
	case layoutSingle:
		switch {
		case o.switches():
			return fmt.Sprintf("single — %d images, new image %s (seed %d)", len(o.ImagePaths), o.holdSummary(), o.Seed)
		case o.Shaded:
			return fmt.Sprintf("single — new shade %s (seed %d)", o.holdSummary(), o.Seed)
		}
	case layoutGrid:
		grid := fmt.Sprintf("grid — %d×%d", o.GridN, o.GridN)
		switch {
		case !o.switches() && o.Shaded:
			return fmt.Sprintf("%s, a new shade in every tile %s (seed %d)", grid, o.holdSummary(), o.Seed)
		case !o.switches():
			return grid
		case o.Tiles == tilesMixed:
			return fmt.Sprintf("%s, %d images mixed across the tiles, new images %s (seed %d)",
				grid, len(o.ImagePaths), o.holdSummary(), o.Seed)
		default:
			return fmt.Sprintf("%s, %d images, same in every tile, new image %s (seed %d)",
				grid, len(o.ImagePaths), o.holdSummary(), o.Seed)
		}
	case layoutTrain:
		rate := fmtFloat(o.Train.Speed) + " per second"
		switch {
		case o.BPM > 0 && o.Train.Beats == 1:
			rate = "one per beat"
		case o.BPM > 0:
			rate = fmt.Sprintf("one every %s beats", fmtFloat(o.Train.Beats))
		}
		if o.Shaded {
			rate += fmt.Sprintf(", a shade per car (seed %d)", o.Seed)
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

// headlessCommand returns the flag-driven invocation equivalent to o. Layout
// flags are included only when they differ from the defaults; the grade,
// motion and seed (when the image order or shades use it) are always included
// so the clip can be reproduced.
func (o *renderOptions) headlessCommand() string {
	d := defaultRenderOptions()
	parts := []string{"nltl-animate"}
	if o.Layout != layoutSingle {
		parts = append(parts, string(o.Layout))
	}
	parts = append(parts, "--grade", o.GradeName, "--motion", o.MotionName)
	if o.BPM > 0 {
		parts = append(parts, "--bpm", strconv.Itoa(o.BPM))
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
	if o.OutputDir != "" && o.outputDirFrom != outdir.EnvVar { // the variable still applies
		parts = append(parts, "--output-dir", o.OutputDir)
	}

	flagIf := func(changed bool, name, value string) {
		if changed {
			parts = append(parts, name, value)
		}
	}
	if o.steps() {
		flagIf(o.Hold != d.Hold, "--hold", strconv.Itoa(o.Hold))
		flagIf(o.HoldBeats != d.HoldBeats, "--hold-beats", fmtFloat(o.HoldBeats))
	}
	flagIf(o.seeded() && o.Seed != 0, "--seed", strconv.FormatUint(o.Seed, 10))
	switch o.Layout {
	case layoutGrid:
		flagIf(true, "--n", strconv.Itoa(o.GridN))
		flagIf(o.switches() && o.Tiles != d.Tiles, "--tiles", o.Tiles)
	case layoutTrain:
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
	if err := o.resolveOutputDir(); err != nil {
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
	g, err := resolveGrade(o.GradeName, cfg)
	if err != nil {
		return err
	}
	m, err := resolveMotion(o.MotionName, cfg)
	if err != nil {
		return err
	}
	o.Shaded = m.Shade > 0
	if o.seeded() && o.Seed == 0 {
		o.Seed = rand.Uint64() | 1 //nolint:gosec // visual randomness; |1 keeps it non-zero
	}

	var duration float64
	if !o.NoAudio {
		duration, err = ffmpeg.GetDuration(o.AudioPath)
		if err != nil {
			return fmt.Errorf("reading audio duration: %w", err)
		}
	}

	ff, err := o.videoInput(m, g)
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
	if s := o.layoutSummary(); s != "" {
		fmt.Println(label("Layout") + valueStyle.Render(s))
	}
	fmt.Println(label("Grade") + valueStyle.Render(g.Name) + labelStyle.Render(" — "+g.Summary()))
	fmt.Println(label("Motion") + valueStyle.Render(m.Name) + labelStyle.Render(" — "+m.Summary()))
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
	fmt.Println(label("Output") + accentStyle.Render(ff.OutputPath))
	fmt.Println()

	return ffmpeg.Render(ff)
}
