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
	"github.com/nothinglefttolearn/nltl-animate/internal/outdir"
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
	NoBPM      bool // "disable": no tempo, nothing follows the beat
	GridN      int
	Tiles      string // grid with several images: "same" or "mixed"
	OutputDir  string // empty: next to the audio file, or the first image
	Preview    bool   // render a preview first (see AfterPreview)
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
	NoBPM         bool
	GridN         int
	Tiles         string
	OutputDir     string // from --output-dir or outdir.EnvVar
	CustomGrades  []grade.Grade
	CustomMotions []motion.Motion

	// Previous, when set, pre-fills every field with what was picked last
	// time instead (going back to the form after a preview).
	Previous *Choices
}

// Run asks for everything with the whole form.
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

	var (
		layoutName = LayoutSingle
		audioPath  = ""
		imagePaths []string
		tiles      = orFirst(d.Tiles, []string{"same", "mixed"})
		gridStr    = strconv.Itoa(max(1, d.GridN))
		gradeName  = d.GradeName
		motionName = d.MotionName
		bpmStr     = bpmField(d.BPM, d.NoBPM)
		outputDir  = d.OutputDir
		preview    = true
	)
	if len(imageFiles) == 1 {
		imagePaths = imageFiles
	}
	if len(audioFiles) > 0 && !d.NoAudio {
		audioPath = audioFiles[0]
	}
	if p := d.Previous; p != nil {
		layoutName, audioPath, imagePaths = p.Layout, p.AudioPath, p.ImagePaths
		tiles, gridStr = p.Tiles, strconv.Itoa(p.GridN)
		gradeName, motionName = p.GradeName, p.MotionName
		outputDir, preview = p.OutputDir, p.Preview
		bpmStr = bpmField(p.BPM, p.NoBPM)
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
				Description("space to toggle, at least 1 — with several, the clip switches between them\n(train: one per car, in random order)").
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
		lookGroup(d, &gradeName, &motionName),
		huh.NewGroup(
			huh.NewInput().
				Title("BPM").
				DescriptionFunc(func() string {
					if audioPath == "" {
						return "Tempo of the motion and switching — blank runs at 120"
					}
					return `Tempo of the motion and switching — blank detects it from the audio,
"disable" ignores the beat`
				}, &audioPath).
				Placeholder("blank, a number like 140, or disable").
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "auto" && audioPath == "" {
						return errors.New(`"auto" needs an audio file — enter a number or leave blank`)
					}
					return validateBPM(s)
				}).
				Value(&bpmStr),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Output directory").
				PlaceholderFunc(func() string {
					near := audioPath
					if near == "" && len(imagePaths) > 0 {
						near = imagePaths[0]
					}
					if near == "" {
						return "blank: next to the input"
					}
					return "blank: next to " + near
				}, []any{&audioPath, &imagePaths}).
				Validate(validateOutputDir).
				Value(&outputDir),
			huh.NewConfirm().
				Title("Preview first?").
				Description("a short low-quality render that opens when it is done, saved as "+PreviewFile).
				Value(&preview),
		),
	)

	if err := runForm(form); err != nil {
		return nil, err
	}

	bpm, autoBPM, noBPM := parseBPM(bpmStr, audioPath != "")
	gridN, _ := strconv.Atoi(gridStr)

	return &Choices{
		Layout:     layoutName,
		AudioPath:  audioPath,
		ImagePaths: imagePaths,
		GradeName:  gradeName,
		MotionName: motionName,
		BPM:        bpm,
		AutoBPM:    autoBPM,
		NoBPM:      noBPM,
		GridN:      gridN,
		Tiles:      tiles,
		OutputDir:  strings.TrimSpace(outputDir),
		Preview:    preview,
	}, nil
}

// RunLook asks only for the grade and motion, starting from c's, and
// changes them in c.
func RunLook(d Defaults, c *Choices) error {
	gradeName, motionName := c.GradeName, c.MotionName
	if err := runForm(huh.NewForm(lookGroup(d, &gradeName, &motionName))); err != nil {
		return err
	}
	c.GradeName, c.MotionName = gradeName, motionName
	return nil
}

// lookGroup is the form page with the grade and the motion. Below each list
// it shows the dial values of the highlighted one.
func lookGroup(d Defaults, gradeName, motionName *string) *huh.Group {
	gradeOpts, gradeNames, gradeSums := namedOptions(d.CustomGrades, grade.Names(), grade.Get,
		func(g grade.Grade) (string, string, string) { return g.Name, g.Description, g.Summary() })
	motionOpts, motionNames, motionSums := namedOptions(d.CustomMotions, motion.Names(), motion.Get,
		func(m motion.Motion) (string, string, string) { return m.Name, m.Description, m.Summary() })
	*gradeName = orFirst(*gradeName, gradeNames)
	*motionName = orFirst(*motionName, motionNames)
	return huh.NewGroup(
		huh.NewSelect[string]().
			Title("Grade").
			DescriptionFunc(func() string {
				return "how the picture looks\n" + gradeSums[*gradeName]
			}, gradeName).
			Options(gradeOpts...).
			Value(gradeName),
		huh.NewSelect[string]().
			Title("Motion").
			DescriptionFunc(func() string {
				return "how the picture moves with the beat (still: none, suggested for photographs)\n" + motionSums[*motionName]
			}, motionName).
			Options(motionOpts...).
			Value(motionName),
	)
}

// PreviewFile is the name of the preview rendered in interactive mode, in
// the output directory. Each preview replaces the last.
const PreviewFile = "nltl_animate_preview.mp4"

// Next is what to do after a preview.
type Next int

const (
	NextFull       Next = iota // render the full clip with the same settings
	NextLook                   // change the grade or motion, then preview again
	NextReseed                 // same settings, a new seed, preview again
	NextEverything             // the whole form again, then preview again
	NextQuit                   // stop, keeping the preview
)

// AfterPreview asks what to do once a preview has rendered. reseed offers a
// new seed, for clips where the seed changes anything. Canceling counts as
// quitting.
func AfterPreview(reseed bool) (Next, error) {
	next := NextFull
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[Next]().
			Title("Preview done").
			Options(afterPreviewOptions(reseed)...).
			Value(&next),
	))
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return NextQuit, nil
		}
		return NextQuit, err
	}
	return next, nil
}

func afterPreviewOptions(reseed bool) []huh.Option[Next] {
	opts := []huh.Option[Next]{
		huh.NewOption("Render the full clip", NextFull),
		huh.NewOption("Change grade or motion", NextLook),
	}
	if reseed {
		opts = append(opts, huh.NewOption("Same settings, new seed (new image order and shades)", NextReseed))
	}
	return append(opts,
		huh.NewOption("Change something else", NextEverything),
		huh.NewOption("Quit (keep the preview)", NextQuit),
	)
}

func runForm(form *huh.Form) error {
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return errors.New("canceled")
		}
		return err
	}
	return nil
}

// validateOutputDir accepts blank (next to the input) or a directory that
// exists and can be written to.
func validateOutputDir(s string) error {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	_, err := outdir.Check(strings.TrimSpace(s))
	return err
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
// values in order, and each one's dial values by name.
func namedOptions[T any](custom []T, builtinNames []string, get func(string) (T, bool),
	describe func(T) (name, description, summary string),
) ([]huh.Option[string], []string, map[string]string) {
	var (
		opts  []huh.Option[string]
		names []string
		sums  = make(map[string]string)
		seen  = make(map[string]bool)
	)
	for _, it := range custom {
		name, desc, sum := describe(it)
		if seen[name] {
			continue
		}
		seen[name] = true
		opts = append(opts, huh.NewOption(fmt.Sprintf("%-14s %s (config)", name, desc), name))
		names = append(names, name)
		sums[name] = sum
	}
	for _, name := range builtinNames {
		if seen[name] {
			continue
		}
		it, _ := get(name)
		_, desc, sum := describe(it)
		opts = append(opts, huh.NewOption(fmt.Sprintf("%-14s %s", name, desc), name))
		names = append(names, name)
		sums[name] = sum
	}
	return opts, names, sums
}

// orFirst returns want if it is one of options, otherwise the first option.
func orFirst(want string, options []string) string {
	if slices.Contains(options, want) {
		return want
	}
	return options[0]
}

// parseBPM reads the BPM field. Blank (or "auto") detects the tempo when
// there is audio to detect it from; without audio there is nothing to detect
// and it runs at 120. "disable" ignores the beat even with audio.
func parseBPM(s string, hasAudio bool) (bpm int, auto, disabled bool) {
	s = strings.TrimSpace(s)
	bpm, _ = strconv.Atoi(s) // 0 for blank, "auto" and "disable"
	disabled = s == "disable"
	return bpm, bpm == 0 && !disabled && hasAudio, disabled
}

// bpmField is the BPM field's starting text: blank detects the tempo (or
// runs at 120 without audio), "disable" ignores the beat.
func bpmField(bpm int, noBPM bool) string {
	switch {
	case noBPM:
		return "disable"
	case bpm > 0:
		return strconv.Itoa(bpm)
	}
	return ""
}

func validateBPM(s string) error {
	switch s = strings.TrimSpace(s); s {
	case "", "auto", "disable":
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return errors.New(`enter a positive number, "disable", or leave blank`)
	}
	return nil
}
