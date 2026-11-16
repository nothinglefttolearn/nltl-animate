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

	"github.com/nothinglefttolearn/nltl-clip/internal/preset"
	"github.com/nothinglefttolearn/nltl-clip/internal/profile"
)

var audioExts = map[string]bool{
	".wav": true, ".mp3": true, ".aiff": true, ".aif": true, ".flac": true,
}

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
}

// Choices holds what the user picked in the form.
type Choices struct {
	AudioPath   string
	ImagePath   string
	ProfileName string
	PresetName  string
	BPM         int
	Still       bool
}

// Defaults pre-fills the form (typically from command-line flags) and supplies
// custom presets loaded from --config so they can be picked alongside built-ins.
type Defaults struct {
	ProfileName   string
	PresetName    string
	BPM           int
	Still         bool
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
		audioPath   = audioFiles[0]
		imagePath   = imageFiles[0]
		profileName = orFirst(d.ProfileName, profile.Names())
		presetName  = orFirst(d.PresetName, presetNames)
		bpmStr      = ""
		still       = d.Still
	)
	if d.BPM > 0 {
		bpmStr = strconv.Itoa(d.BPM)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Audio file").
				Options(fileOptions(audioFiles)...).
				Value(&audioPath),
			huh.NewSelect[string]().
				Title("Image file").
				Options(fileOptions(imageFiles)...).
				Value(&imagePath),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Profile").
				Options(profileOptions()...).
				Value(&profileName),
			huh.NewSelect[string]().
				Title("Preset").
				Options(presetOpts...).
				Value(&presetName),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("BPM").
				Description("Tempo-synced animations — leave blank to disable").
				Placeholder("e.g. 140").
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

	bpm := 0
	if bpmStr != "" {
		bpm, _ = strconv.Atoi(bpmStr)
	}

	return &Choices{
		AudioPath:   audioPath,
		ImagePath:   imagePath,
		ProfileName: profileName,
		PresetName:  presetName,
		BPM:         bpm,
		Still:       still,
	}, nil
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

func profileOptions() []huh.Option[string] {
	names := profile.Names()
	opts := make([]huh.Option[string], len(names))
	for i, name := range names {
		p, _ := profile.Get(name)
		label := fmt.Sprintf("%-22s %dx%d", name, p.Width, p.Height)
		opts[i] = huh.NewOption(label, name)
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
	if s == "" {
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return errors.New("enter a positive number or leave blank")
	}
	return nil
}
