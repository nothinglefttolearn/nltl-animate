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

	"github.com/nothinglefttolearn/nltl-animate/internal/preset"
	"github.com/nothinglefttolearn/nltl-animate/internal/style"
)

var audioExts = map[string]bool{
	".wav": true, ".mp3": true, ".aiff": true, ".aif": true, ".flac": true,
}

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
}

// Style names match the CLI subcommands ("single" is the root command).
const (
	StyleSingle = "single"
	StyleGrid   = "grid"
	StyleTrain  = "train"
)

// Choices holds what the user picked in the form.
type Choices struct {
	Style      string
	AudioPath  string
	ImagePaths []string
	PresetName string
	BPM        int
	AutoBPM    bool
	Still      bool
	GridN      int
	Tiles      string // grid with several images: "same" or "mixed"
}

// Defaults pre-fills the form (typically from command-line flags) and supplies
// custom presets loaded from --config so they can be picked alongside built-ins.
type Defaults struct {
	PresetName    string
	BPM           int
	AutoBPM       bool
	Still         bool
	GridN         int
	Tiles         string
	CustomPresets []preset.Preset
}

func Run(d Defaults) (*Choices, error) {
	audioFiles, err := scanFiles(audioExts)
	if err != nil {
		return nil, err
	}
	if len(audioFiles) == 0 {
		return nil, errors.New("no audio files found in current directory")
	}

	imageFiles, err := scanFiles(imageExts)
	if err != nil {
		return nil, err
	}
	if len(imageFiles) == 0 {
		return nil, errors.New("no image files found in current directory")
	}

	presetOpts, presetNames := presetOptions(d.CustomPresets)

	var (
		styleName  = StyleSingle
		audioPath  = audioFiles[0]
		imagePaths []string
		tiles      = orFirst(d.Tiles, []string{"same", "mixed"})
		gridStr    = strconv.Itoa(max(1, d.GridN))
		presetName = orFirst(d.PresetName, presetNames)
		bpmStr     = ""
		still      = d.Still
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
	isNot := func(s string) func() bool { return func() bool { return styleName != s } }

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Style").
				Options(styleOptions()...).
				Value(&styleName),
			huh.NewSelect[string]().
				Title("Audio file").
				Options(fileOptions(audioFiles)...).
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
				Description(fmt.Sprintf("tiles per side, 1–%d", style.MaxGrid)).
				Validate(validateGridN).
				Value(&gridStr),
		).WithHideFunc(isNot(StyleGrid)),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Tiles").
				Options(
					huh.NewOption("same    every tile shows the same image, switching together", "same"),
					huh.NewOption("mixed   a different image in each tile, each switching on its own", "mixed"),
				).
				Value(&tiles),
		).WithHideFunc(func() bool { return styleName != StyleGrid || len(imagePaths) < 2 }),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Preset").
				Options(presetOpts...).
				Value(&presetName),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("BPM").
				Description("Tempo-synced animations — \"auto\" detects it, blank disables").
				Placeholder("e.g. 140, or auto").
				Validate(validateBPM).
				Value(&bpmStr),
			huh.NewConfirm().
				Title("Disable motion effects?").
				Description("Suggested for photograph inputs").
				Value(&still),
		),
	)

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return nil, errors.New("cancelled")
		}
		return nil, err
	}

	bpm, autoBPM := 0, bpmStr == "auto"
	if bpmStr != "" && !autoBPM {
		bpm, _ = strconv.Atoi(bpmStr)
	}
	gridN, _ := strconv.Atoi(gridStr)

	return &Choices{
		Style:      styleName,
		AudioPath:  audioPath,
		ImagePaths: imagePaths,
		PresetName: presetName,
		BPM:        bpm,
		AutoBPM:    autoBPM,
		Still:      still,
		GridN:      gridN,
		Tiles:      tiles,
	}, nil
}

// styleOptions lists the styles.
func styleOptions() []huh.Option[string] {
	return []huh.Option[string]{
		huh.NewOption("single   the image gently moving (several: switching between them)", StyleSingle),
		huh.NewOption("grid     the image tiled N×N", StyleGrid),
		huh.NewOption("train    copies fly past on the left", StyleTrain),
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
	if err != nil || n < 1 || n > style.MaxGrid {
		return fmt.Errorf("enter a number from 1 to %d", style.MaxGrid)
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

func fileOptions(files []string) []huh.Option[string] {
	opts := make([]huh.Option[string], len(files))
	for i, f := range files {
		opts[i] = huh.NewOption(f, f)
	}
	return opts
}

// presetOptions lists custom presets first, then built-ins not shadowed by a
// custom preset of the same name. It also returns the option values in order.
func presetOptions(custom []preset.Preset) ([]huh.Option[string], []string) {
	var (
		opts  []huh.Option[string]
		names []string
		seen  = make(map[string]bool)
	)
	for _, p := range custom {
		if seen[p.Name] {
			continue
		}
		seen[p.Name] = true
		label := fmt.Sprintf("%-14s %s (config)", p.Name, p.Description)
		opts = append(opts, huh.NewOption(label, p.Name))
		names = append(names, p.Name)
	}
	for _, name := range preset.Names() {
		if seen[name] {
			continue
		}
		p, _ := preset.Get(name)
		label := fmt.Sprintf("%-14s %s", name, p.Description)
		opts = append(opts, huh.NewOption(label, name))
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
