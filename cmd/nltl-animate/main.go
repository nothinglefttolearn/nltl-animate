package main

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/nothinglefttolearn/nltl-animate/internal/grade"
	"github.com/nothinglefttolearn/nltl-animate/internal/interactive"
	"github.com/nothinglefttolearn/nltl-animate/internal/motion"
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
in a row), every beat with --bpm or as set by --hold/--hold-beats. With a
motion that has a tint (e.g. --motion shifting), every image shown gets its
own color, so even a single image changes color on every switch. Every run
prints its --seed so a clip can be reproduced exactly.

With --no-audio, every argument is an image and the clip is silent and loops
seamlessly: it lasts one pass through the images (each shown once, in a
shuffled order that repeats), or 3 seconds with a single image.

Every clip has a layout, a grade and a motion, chosen separately:
  layout   how the images are arranged: single (this command), or the
           grid and train subcommands
  --grade  how the picture looks (color, softness, vignette, grain)
  --motion how the picture moves with the beat (breath, shake, pulse, and
           tint: a different color for every image shown)
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
			return runInteractive(opts)
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

func runInteractive(opts renderOptions) error {
	cfg, err := loadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	choices, err := interactive.Run(interactive.Defaults{
		NoAudio:       opts.NoAudio,
		GradeName:     opts.GradeName,
		MotionName:    opts.MotionName,
		BPM:           opts.BPM,
		AutoBPM:       opts.AutoBPM,
		GridN:         opts.GridN,
		Tiles:         opts.Tiles,
		CustomGrades:  cfg.Grades,
		CustomMotions: cfg.Motions,
	})
	if err != nil {
		return err
	}
	opts.Layout = layoutName(choices.Layout)
	opts.AudioPath = choices.AudioPath
	opts.NoAudio = choices.AudioPath == ""
	opts.ImagePaths = choices.ImagePaths
	opts.GradeName, opts.MotionName = choices.GradeName, choices.MotionName
	opts.BPM, opts.AutoBPM = choices.BPM, choices.AutoBPM
	opts.GridN = choices.GridN
	opts.Tiles = choices.Tiles

	fmt.Println()
	if err := renderClip(&opts, cfg); err != nil {
		return err
	}
	fmt.Printf("\nTo run this again without prompts:\n  %s\n", opts.headlessCommand())
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
frame, rates are in beats (at 120 BPM without --bpm):
  breath  how far the frame zooms in and out, once every breath_beats
  shake   how far the frame drifts and jitters
  pulse   how much the brightness swells, once every pulse_beats
  tint    how far each image shown (each switch, grid tile or train car)
          leans toward its own color, without getting brighter or darker`,
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
