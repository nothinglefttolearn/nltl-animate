package interactive

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"

	"github.com/nothinglefttolearn/nltl-animate/internal/grade"
	"github.com/nothinglefttolearn/nltl-animate/internal/layout"
	"github.com/nothinglefttolearn/nltl-animate/internal/motion"
)

var audioExts = map[string]bool{
	".wav": true, ".mp3": true, ".aiff": true, ".aif": true, ".flac": true,
}

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
}

// Layout names match the CLI subcommands ("single" is the root command).
const (
	LayoutSingle = "single"
	LayoutGrid   = "grid"
	LayoutTrain  = "train"
)

// Choices holds what the user picked in the form.
type Choices struct {
	Layout     string
	AudioPath  string // empty for a silent loop
	ImagePaths []string
	GradeName  string
	MotionName string
	BPM        int
	AutoBPM    bool
	GridN      int
	Tiles      string // grid with several images: "same" or "mixed"
}

// Defaults pre-fills the form (typically from command-line flags) and supplies
// custom grades and motions loaded from --config so they can be picked
// alongside the built-ins.
type Defaults struct {
	NoAudio       bool
	GradeName     string
	MotionName    string
	BPM           int
	AutoBPM       bool
	GridN         int
	Tiles         string
	CustomGrades  []grade.Grade
	CustomMotions []motion.Motion
}

func Run(d Defaults) (*Choices, error) {
	audioFiles, err := scanFiles(audioExts)
	if err != nil {
		return nil, err
	}
	imageFiles, err := scanFiles(imageExts)
	if err != nil {
		return nil, err
	}
	if len(imageFiles) == 0 {
		return nil, errors.New("no image files found in current directory")
	}

	gradeOpts, gradeNames := namedOptions(d.CustomGrades, grade.Names(), grade.Get,
		func(g grade.Grade) (string, string) { return g.Name, g.Description })
	motionOpts, motionNames := namedOptions(d.CustomMotions, motion.Names(), motion.Get,
		func(m motion.Motion) (string, string) { return m.Name, m.Description })

	var (
		layoutName = LayoutSingle
		audioPath  = ""
		imagePaths []string
		tiles      = orFirst(d.Tiles, []string{"same", "mixed"})
		gridStr    = strconv.Itoa(max(1, d.GridN))
		gradeName  = orFirst(d.GradeName, gradeNames)
		motionName = orFirst(d.MotionName, motionNames)
		bpmStr     = ""
	)
	switch {
	case d.AutoBPM:
		bpmStr = "auto"
	case d.BPM > 0:
		bpmStr = strconv.Itoa(d.BPM)
	}
	if len(imageFiles) == 1 {
		imagePaths = imageFiles
	}
	if len(audioFiles) > 0 && !d.NoAudio {
		audioPath = audioFiles[0]
	}
	isNot := func(s string) func() bool { return func() bool { return layoutName != s } }

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Layout").
				Options(layoutOptions()...).
				Value(&layoutName),
			huh.NewSelect[string]().
				Title("Audio file").
				Options(audioOptions(audioFiles)...).
				Value(&audioPath),
		),
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Images").
				Description("space to toggle, at least 1 — with several, the clip switches between them\n(train: one per car, in order)").
				Options(fileOptions(imageFiles)...).
				Validate(validateImages).
				Value(&imagePaths),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Grid size").
				Description(fmt.Sprintf("tiles per side, 1–%d", layout.MaxGrid)).
				Validate(validateGridN).
				Value(&gridStr),
		).WithHideFunc(isNot(LayoutGrid)),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Tiles").
				Options(
					huh.NewOption("same    every tile shows the same image, switching together", "same"),
					huh.NewOption("mixed   a different image in each tile, each switching on its own", "mixed"),
				).
				Value(&tiles),
		).WithHideFunc(func() bool { return layoutName != LayoutGrid || len(imagePaths) < 2 }),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Grade").
				Description("how the picture looks").
				Options(gradeOpts...).
				Value(&gradeName),
			huh.NewSelect[string]().
				Title("Motion").
				Description("how the picture moves with the beat (still: none, suggested for photographs)").
				Options(motionOpts...).
				Value(&motionName),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("BPM").
				Description("Tempo-synced animations — \"auto\" detects it, blank disables").
				Placeholder("e.g. 140, or auto").
				Validate(func(s string) error {
					if s == "auto" && audioPath == "" {
						return errors.New(`"auto" needs an audio file — enter a number or leave blank`)
					}
					return validateBPM(s)
				}).
				Value(&bpmStr),
		),
	)

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errors.New("canceled")
		}
		return nil, err
	}

	bpm, autoBPM := 0, bpmStr == "auto"
	if bpmStr != "" && !autoBPM {
		bpm, _ = strconv.Atoi(bpmStr)
	}
	gridN, _ := strconv.Atoi(gridStr)

	return &Choices{
		Layout:     layoutName,
		AudioPath:  audioPath,
		ImagePaths: imagePaths,
		GradeName:  gradeName,
		MotionName: motionName,
		BPM:        bpm,
		AutoBPM:    autoBPM,
		GridN:      gridN,
		Tiles:      tiles,
	}, nil
}

// layoutOptions lists the layouts.
func layoutOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("single   the image filling the frame (several: switching between them)", LayoutSingle),
		huh.NewOption("grid     the image tiled N×N", LayoutGrid),
		huh.NewOption("train    copies fly past on the left", LayoutTrain),
	}
}

func validateImages(paths []string) error {
	if len(paths) < 1 {
		return errors.New("pick at least 1 image")
	}
	return nil
}

func validateGridN(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > layout.MaxGrid {
		return fmt.Errorf("enter a number from 1 to %d", layout.MaxGrid)
	}
	return nil
}

func scanFiles(exts map[string]bool) ([]string, error) {
	entries, err := os.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && exts[strings.ToLower(filepath.Ext(e.Name()))] {
			files = append(files, e.Name())
		}
	}
	return files, nil
}

// audioOptions lists the audio files, then a silent loop (value "").
func audioOptions(files []string) []huh.Option[string] {
	return append(fileOptions(files), huh.NewOption("none — silent clip that loops seamlessly", ""))
}

func fileOptions(files []string) []huh.Option[string] {
	opts := make([]huh.Option[string], len(files))
	for i, f := range files {
		opts[i] = huh.NewOption(f, f)
	}
	return opts
}

// namedOptions lists custom grades or motions first, then built-ins not
// shadowed by a custom one of the same name. It also returns the option
// values in order.
func namedOptions[T any](custom []T, builtinNames []string, get func(string) (T, bool),
	describe func(T) (name, description string),
) ([]huh.Option[string], []string) {
	var (
		opts  []huh.Option[string]
		names []string
		seen  = make(map[string]bool)
	)
	for _, it := range custom {
		name, desc := describe(it)
		if seen[name] {
			continue
		}
		seen[name] = true
		opts = append(opts, huh.NewOption(fmt.Sprintf("%-14s %s (config)", name, desc), name))
		names = append(names, name)
	}
	for _, name := range builtinNames {
		if seen[name] {
			continue
		}
		it, _ := get(name)
		_, desc := describe(it)
		opts = append(opts, huh.NewOption(fmt.Sprintf("%-14s %s", name, desc), name))
		names = append(names, name)
	}
	return opts, names
}

// orFirst returns want if it is one of options, otherwise the first option.
func orFirst(want string, options []string) string {
	if slices.Contains(options, want) {
		return want
	}
	return options[0]
}

func validateBPM(s string) error {
	if s == "" || s == "auto" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return errors.New(`enter a positive number, "auto", or leave blank`)
	}
	return nil
}
