package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/nothinglefttolearn/nltl-animate/internal/grade"
	"github.com/nothinglefttolearn/nltl-animate/internal/interactive"
	"github.com/nothinglefttolearn/nltl-animate/internal/motion"
	"github.com/nothinglefttolearn/nltl-animate/internal/outdir"
)

var (
	labelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	valueStyle  = lipgloss.NewStyle().Bold(true)
	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
)

// version can be set at build time:
//
//	go build -ldflags "-X main.version=v1.2.3" ./cmd/nltl-animate
var version string

// buildVersion prefers an injected version, then the module version Go
// records at build time (a tag for `go install ...@vX`, a VCS pseudo-version
// for local builds).
func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func label(s string) string { return labelStyle.Render(fmt.Sprintf("%-10s", s)) }

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	opts := defaultRenderOptions()

	root := &cobra.Command{
		Use:   "nltl-animate [audio] [image...]",
		Short: "Animate images to music",
		Long: `Animate images to music: renders a square 1080×1080 video from an audio
track and one or more images, moving and switching them in time with the beat.

With an audio file and an image, renders the image gently moving. With
several images, switches between them at random (never the same image twice
in a row), every beat or as set by --hold/--hold-beats. The tempo is
detected from the audio unless --bpm gives it. With a motion that has a
shade (e.g. --motion shifting), every image shown is a little lighter or
darker than the last, so even a single image changes on every switch. Every
run prints its --seed so a clip can be reproduced exactly.

With --no-audio, every argument is an image and the clip is silent and loops
seamlessly: it lasts one pass through the images (each shown once, in a
shuffled order that repeats), or 3 seconds with a single image.

Every clip has a layout, a grade and a motion, chosen separately:
  layout   how the images are arranged: single (this command), or the
           grid and train subcommands
  --grade  how the picture looks (color, softness, vignette, grain)
  --motion how the picture moves with the beat (breath, shake, pulse, and
           shade: every image shown a little lighter or darker)
Run "nltl-animate grades" and "nltl-animate motions" to see what each does.

Every layout takes one or more images. With no arguments, an interactive
form asks for everything.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && args[0] == "cycle" {
				return errors.New("cycle is no longer a separate command: pass several images directly, e.g. nltl-animate song.wav a.png b.png")
			}
			if len(args) == 0 || len(args) >= opts.minArgs() {
				return nil
			}
			return fmt.Errorf("expected no args (interactive) or audio and image(s), got %d arg — pass --no-audio to make a silent loop from images alone", len(args))
		},
		Version: buildVersion(),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Args and flags parsed fine, so any error from here on is a
			// runtime failure (bad config, ffmpeg) and usage text is noise.
			cmd.SilenceUsage = true

			if len(args) > 0 {
				opts.Layout = layoutSingle
				opts.setArgs(args)
				return run(opts)
			}
			return runInteractive(&opts)
		},
	}
	addRenderFlags(root, &opts)
	addSwitchFlags(root, &opts)

	root.AddCommand(
		gridCmd(), trainCmd(),
		gradesCmd(), motionsCmd(),
	)
	return root
}

func runInteractive(opts *renderOptions) error {
	cfg, err := loadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	flagDir := opts.OutputDir
	startDir, _ := outdir.Resolve(flagDir)
	d := interactive.Defaults{
		NoAudio:       opts.NoAudio,
		GradeName:     opts.GradeName,
		MotionName:    opts.MotionName,
		BPM:           opts.BPM,
		AutoBPM:       opts.AutoBPM,
		GridN:         opts.GridN,
		Tiles:         opts.Tiles,
		OutputDir:     startDir,
		CustomGrades:  cfg.Grades,
		CustomMotions: cfg.Motions,
	}
	choices, err := askAll(d)
	if err != nil {
		return err
	}
	apply := func() {
		opts.Layout = layoutName(choices.Layout)
		opts.AudioPath = choices.AudioPath
		opts.NoAudio = choices.AudioPath == ""
		opts.ImagePaths = choices.ImagePaths
		opts.GradeName, opts.MotionName = choices.GradeName, choices.MotionName
		opts.BPM, opts.AutoBPM = choices.BPM, choices.AutoBPM
		opts.GridN = choices.GridN
		opts.Tiles = choices.Tiles
		opts.OutputDir, opts.outputDirFrom = choices.OutputDir, outputDirSource(choices.OutputDir, flagDir)
	}
	apply()

	full := func() error {
		opts.Preview, opts.OutputFile, opts.Overwrite = false, "", false
		fmt.Println()
		if err := renderClip(opts, cfg); err != nil {
			return err
		}
		fmt.Printf("\nTo run this again without prompts:\n  %s\n", opts.headlessCommand())
		return nil
	}
	if !choices.Preview {
		return full()
	}

	for {
		opts.Preview, opts.OutputFile, opts.Overwrite = true, interactive.PreviewFile, true
		fmt.Println()
		if err := renderClip(opts, cfg); err != nil {
			return err
		}
		previewPath := opts.outputPath(time.Time{})
		if err := openFile(previewPath); err != nil {
			fmt.Println(warnStyle.Render("Could not open the preview: ") + err.Error())
		}
		fmt.Printf("\nTo run this preview again without prompts:\n  %s\n\n", opts.headlessCommand())

		next, err := askNext(opts.seeded())
		if err != nil {
			return err
		}
		switch next {
		case interactive.NextFull:
			if err := os.Remove(previewPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("deleting the preview: %w", err)
			}
			return full()
		case interactive.NextLook:
			if err := askLook(d, choices); err != nil {
				return err
			}
			opts.GradeName, opts.MotionName = choices.GradeName, choices.MotionName
		case interactive.NextReseed:
			opts.Seed = 0 // renderClip picks a new one
		case interactive.NextEverything:
			d.Previous = choices
			if choices, err = askAll(d); err != nil {
				return err
			}
			apply()
			if !choices.Preview {
				if err := os.Remove(previewPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf("deleting the preview: %w", err)
				}
				return full()
			}
		case interactive.NextQuit:
			fmt.Println(labelStyle.Render("Preview kept at ") + accentStyle.Render(previewPath))
			return nil
		}
	}
}

// The interactive questions; tests replace them.
var (
	askAll  = interactive.Run
	askLook = interactive.RunLook
	askNext = interactive.AfterPreview
)

// outputDirSource names where the form's output directory came from: the
// environment variable when it is unchanged from it, otherwise the flag's
// name (also for a directory typed in the form). A cleared field is the
// form's own choice to write next to the input, which the variable must not
// override.
func outputDirSource(dir, flagDir string) string {
	switch {
	case dir == "":
		return formSource
	case flagDir == "" && dir == os.Getenv(outdir.EnvVar):
		return outdir.EnvVar
	default:
		return outdir.FlagSource
	}
}

// formSource marks an output directory left blank in the form.
const formSource = "the form"

// openFile opens path in the default app, without waiting for it. Tests
// replace it.
var openFile = func(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func gradesCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "grades",
		Short: "List the grades (how the picture looks) and their dial values",
		Long: `Lists every grade with its dial values. Grade dials are percentages, and 0
leaves the image untouched:
  contrast    % change: +15 is punchier, -20 flatter
  saturation  % change: -100 is black and white
  softness    blur: 0 sharp, 100 heavily smeared
  vignette    corner darkening: 0 none, 100 strongest
  grain       film grain: 0 clean, 100 very heavy`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}
			var rows []listRow
			for _, g := range cfg.Grades {
				rows = append(rows, listRow{g.Name, g.Description, g.Summary(), true})
			}
			for _, name := range grade.Names() {
				g, _ := grade.Get(name)
				rows = append(rows, listRow{name, g.Description, g.Summary(), false})
			}
			printList("Grades", grade.Default, rows)
			return nil
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "YAML config file with custom grades and motions")
	return cmd
}

func motionsCmd() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "motions",
		Short: "List the motions (how the picture moves with the beat) and their dial values",
		Long: `Lists every motion with its dial values. Strengths are percentages of the
frame, rates are in beats (at the track's detected tempo without --bpm, or
120 BPM for a silent loop without --bpm):
  breath  how far the frame zooms in and out, once every breath_beats
  shake   how far the frame drifts and jitters
  pulse   how much the brightness swells, once every pulse_beats
  shade   how much lighter or darker each image shown (each switch, grid
          tile or train car) can be, picked at random`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			cfg, err := loadConfig(configPath)
			if err != nil {
				return err
			}
			var rows []listRow
			for _, m := range cfg.Motions {
				rows = append(rows, listRow{m.Name, m.Description, m.Summary(), true})
			}
			for _, name := range motion.Names() {
				m, _ := motion.Get(name)
				rows = append(rows, listRow{name, m.Description, m.Summary(), false})
			}
			printList("Motions", motion.Default, rows)
			return nil
		},
	}
	cmd.Flags().StringVarP(&configPath, "config", "c", "", "YAML config file with custom grades and motions")
	return cmd
}

// listRow is one grade or motion in a listing.
type listRow struct {
	name, description, summary string
	custom                     bool
}

// printList prints custom entries first, then built-ins not replaced by a
// custom one of the same name.
func printList(title, defaultName string, rows []listRow) {
	fmt.Println(valueStyle.Render(title + ":"))
	seen := make(map[string]bool)
	for _, r := range rows {
		if seen[r.name] {
			continue
		}
		seen[r.name] = true
		var notes []string
		if r.custom {
			notes = append(notes, "from --config")
		}
		if r.name == defaultName {
			notes = append(notes, "default")
		}
		desc := r.description
		if len(notes) > 0 {
			desc = strings.TrimSpace(desc + " (" + strings.Join(notes, ", ") + ")")
		}
		fmt.Printf("  %s  %s\n  %s  %s\n",
			valueStyle.Render(fmt.Sprintf("%-10s", r.name)), desc,
			strings.Repeat(" ", 10), labelStyle.Render(r.summary))
	}
}
