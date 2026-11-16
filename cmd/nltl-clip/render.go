package main

import (
	"errors"
	"fmt"
	"image"
	"math/rand/v2"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nothinglefttolearn/nltl-clip/internal/config"
	"github.com/nothinglefttolearn/nltl-clip/internal/ffmpeg"
	"github.com/nothinglefttolearn/nltl-clip/internal/frames"
	"github.com/nothinglefttolearn/nltl-clip/internal/preset"
	"github.com/nothinglefttolearn/nltl-clip/internal/profile"
	"github.com/nothinglefttolearn/nltl-clip/internal/style"
	"github.com/nothinglefttolearn/nltl-clip/internal/video"
)

type clipStyle string

const (
	styleSingle clipStyle = "single"
	styleCycle  clipStyle = "cycle"
	styleGrid   clipStyle = "grid"
	styleTrain  clipStyle = "train"
)

// renderOptions is everything needed to render one clip. It is filled from
// flags, and in interactive mode the TUI overrides the fields it asks about.
type renderOptions struct {
	Style       clipStyle
	AudioPath   string
	ImagePaths  []string // exactly one, except for cycle (two or more)
	ProfileName string
	PresetName  string
	BPM         int
	Preview     bool
	Still       bool
	Verbose     bool
	OutputDir   string
	ConfigPath  string

	// cycle
	Hold      int     // frames per image
	HoldBeats float64 // beats per image; overrides Hold when > 0
	Seed      uint64  // 0 = pick one at render time

	// grid
	GridN int

	// train (Train.BPM and Train.BG are filled from BPM and TrainBG)
	Train   style.TrainOptions
	TrainBG string
}

func defaultRenderOptions() renderOptions {
	return renderOptions{
		Style:       styleSingle,
		ProfileName: "instagram-square",
		PresetName:  "industrial",
		Hold:        1,
		GridN:       3,
		Train:       style.DefaultTrainOptions(),
		TrainBG:     "#000000",
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

	wantImages := "exactly 1 image"
	okImages := len(o.ImagePaths) == 1
	if o.Style == styleCycle {
		wantImages, okImages = "at least 2 images", len(o.ImagePaths) >= 2
	}
	if !okImages {
		return fmt.Errorf("%s style needs %s, got %d", o.Style, wantImages, len(o.ImagePaths))
	}

	switch o.Style {
	case styleSingle:
	case styleCycle:
		if o.Hold < 1 {
			return fmt.Errorf("--hold must be at least 1 frame, got %d", o.Hold)
		}
		if o.HoldBeats < 0 {
			return fmt.Errorf("--hold-beats must be positive, got %g", o.HoldBeats)
		}
		if o.HoldBeats > 0 && o.BPM == 0 {
			return errors.New("--hold-beats needs --bpm")
		}
		if o.HoldBeats > 0 && o.Hold != 1 {
			return errors.New("use either --hold or --hold-beats, not both")
		}
	case styleGrid:
		if o.GridN < 1 || o.GridN > style.MaxGrid {
			return fmt.Errorf("--n must be between 1 and %d, got %d", style.MaxGrid, o.GridN)
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

// holdFrames is the cycle hold length in frames.
func (o *renderOptions) holdFrames() int {
	if o.HoldBeats > 0 {
		return style.HoldFromBeats(o.HoldBeats, o.BPM)
	}
	return o.Hold
}

func (o *renderOptions) outputPath(prof profile.Profile) string {
	audio := o.AudioPath
	base := strings.TrimSuffix(filepath.Base(audio), filepath.Ext(audio))
	dir := o.OutputDir
	if dir == "" {
		dir = filepath.Dir(audio)
	}

	suffix := prof.Suffix
	switch o.Style {
	case styleSingle:
	case styleGrid:
		suffix += "_grid" + strconv.Itoa(o.GridN)
	default:
		suffix += "_" + string(o.Style)
	}
	if o.Preview {
		suffix += "_preview"
	}
	return filepath.Join(dir, fmt.Sprintf("%s_%s.mp4", base, suffix))
}

// videoInput returns the ffmpeg video input (still image or frame source)
// and filtergraph for the style.
func (o *renderOptions) videoInput(p preset.Preset, prof profile.Profile) (ffmpeg.Options, error) {
	w, h := prof.Width, prof.Height
	switch o.Style {
	case styleGrid:
		return ffmpeg.Options{ImagePath: o.ImagePaths[0], Filter: style.GridFilter(p, w, h, o.GridN, o.BPM)}, nil

	case styleCycle:
		// Decode and shrink one image at a time so large originals never
		// all sit in memory together.
		imgs := make([]*image.RGBA, 0, len(o.ImagePaths))
		for _, path := range o.ImagePaths {
			img, err := frames.Load(path)
			if err != nil {
				return ffmpeg.Options{}, err
			}
			imgs = append(imgs, frames.Cover(img, w, h))
		}
		src, err := style.NewCycle(imgs, o.holdFrames(), o.Seed)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		return ffmpeg.Options{Frames: src, Filter: p.BuildFilter(w, h, o.BPM)}, nil

	case styleTrain:
		img, err := frames.Load(o.ImagePaths[0])
		if err != nil {
			return ffmpeg.Options{}, err
		}
		src, err := style.NewTrain(img, w, h, o.Train)
		if err != nil {
			return ffmpeg.Options{}, err
		}
		return ffmpeg.Options{Frames: src, Filter: p.BuildFilter(w, h, o.BPM)}, nil

	default:
		return ffmpeg.Options{ImagePath: o.ImagePaths[0], Filter: p.BuildFilter(w, h, o.BPM)}, nil
	}
}

// styleSummary describes the style settings for the pre-render summary.
func (o *renderOptions) styleSummary() string {
	switch o.Style {
	case styleCycle:
		every := "every frame"
		switch {
		case o.HoldBeats > 0:
			every = fmt.Sprintf("every %s beats", fmtFloat(o.HoldBeats))
		case o.Hold > 1:
			every = fmt.Sprintf("every %d frames", o.Hold)
		}
		return fmt.Sprintf("cycle — %d images, new image %s (seed %d)", len(o.ImagePaths), every, o.Seed)
	case styleGrid:
		return fmt.Sprintf("grid — %d×%d", o.GridN, o.GridN)
	case styleTrain:
		rate := fmtFloat(o.Train.Speed) + " per second"
		switch {
		case o.BPM > 0 && o.Train.Beats == 1:
			rate = "one per beat"
		case o.BPM > 0:
			rate = fmt.Sprintf("one every %s beats", fmtFloat(o.Train.Beats))
		}
		return fmt.Sprintf("train — %d copies, %s", o.Train.Copies, rate)
	}
	return ""
}

// headlessCommand returns the flag-driven invocation equivalent to o. Style
// flags are included only when they differ from the defaults; the cycle seed
// is always included so the clip can be reproduced.
func (o *renderOptions) headlessCommand() string {
	d := defaultRenderOptions()
	parts := []string{"nltl-clip"}
	if o.Style != styleSingle {
		parts = append(parts, string(o.Style))
	}
	parts = append(parts, "--profile", o.ProfileName, "--preset", o.PresetName)
	if o.BPM > 0 {
		parts = append(parts, "--bpm", strconv.Itoa(o.BPM))
	}
	if o.Still {
		parts = append(parts, "--still")
	}
	if o.Preview {
		parts = append(parts, "--preview")
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
	switch o.Style {
	case styleCycle:
		flagIf(o.Hold != d.Hold, "--hold", strconv.Itoa(o.Hold))
		flagIf(o.HoldBeats != d.HoldBeats, "--hold-beats", fmtFloat(o.HoldBeats))
		flagIf(o.Seed != 0, "--seed", strconv.FormatUint(o.Seed, 10))
	case styleGrid:
		flagIf(true, "--n", strconv.Itoa(o.GridN))
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

	parts = append(parts, o.AudioPath)
	parts = append(parts, o.ImagePaths...)
	return strings.Join(parts, " ")
}

func fmtFloat(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }

func renderClip(o *renderOptions, cfg *config.Config) error {
	if err := o.validate(); err != nil {
		return err
	}
	if o.Style == styleCycle && o.Seed == 0 {
		o.Seed = rand.Uint64() | 1 //nolint:gosec // visual randomness; |1 keeps it non-zero
	}

	prof, ok := profile.Get(o.ProfileName)
	if !ok {
		return fmt.Errorf("unknown profile %q — try: %s", o.ProfileName, strings.Join(profile.Names(), ", "))
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

	duration, err := ffmpeg.GetDuration(o.AudioPath)
	if err != nil {
		return fmt.Errorf("reading audio duration: %w", err)
	}

	ff, err := o.videoInput(p, prof)
	if err != nil {
		return err
	}
	ff.AudioPath = o.AudioPath
	ff.OutputPath = o.outputPath(prof)
	ff.Duration = duration
	ff.Preview = o.Preview
	ff.Verbose = o.Verbose

	fmt.Println()
	if s := o.styleSummary(); s != "" {
		fmt.Println(label("Style") + valueStyle.Render(s))
	}
	fmt.Println(label("Profile") + valueStyle.Render(fmt.Sprintf("%s (%dx%d)", prof.Name, prof.Width, prof.Height)))
	fmt.Println(label("Preset") + valueStyle.Render(fmt.Sprintf("%s — %s", p.Name, p.Description)))
	if o.BPM > 0 {
		fmt.Println(label("BPM") + valueStyle.Render(strconv.Itoa(o.BPM)))
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
