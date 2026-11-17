package main

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/nothinglefttolearn/nltl-animate/internal/interactive"
	"github.com/nothinglefttolearn/nltl-animate/internal/preset"
)

var (
	labelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	valueStyle  = lipgloss.NewStyle().Bold(true)
	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
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
in a row), every beat with --bpm or as set by --hold/--hold-beats; every run
prints its --seed so a clip can be reproduced exactly.

With --no-audio, every argument is an image and the clip is silent and loops
seamlessly: it lasts one pass through the images (each shown once, in a
shuffled order that repeats), or 3 seconds with a single image.

Other styles are subcommands (grid, train); every style takes one or more
images. With no arguments, an interactive form asks for everything.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && args[0] == "cycle" {
				return errors.New("the cycle style is now the default: pass several images directly, e.g. nltl-animate song.wav a.png b.png")
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
				opts.Style = styleSingle
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
		presetsCmd(),
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
		PresetName:    opts.PresetName,
		BPM:           opts.BPM,
		AutoBPM:       opts.AutoBPM,
		Still:         opts.Still,
		GridN:         opts.GridN,
		Tiles:         opts.Tiles,
		CustomPresets: cfg.Presets,
	})
	if err != nil {
		return err
	}
	opts.Style = clipStyle(choices.Style)
	opts.AudioPath = choices.AudioPath
	opts.NoAudio = choices.AudioPath == ""
	opts.ImagePaths = choices.ImagePaths
	opts.PresetName = choices.PresetName
	opts.BPM, opts.AutoBPM = choices.BPM, choices.AutoBPM
	opts.Still = choices.Still
	opts.GridN = choices.GridN
	opts.Tiles = choices.Tiles

	fmt.Println()
	if err := renderClip(&opts, cfg); err != nil {
		return err
	}
	fmt.Printf("\nTo run this again without prompts:\n  %s\n", opts.headlessCommand())
	return nil
}

func presetsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "presets",
		Short: "List available built-in presets",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(valueStyle.Render("Built-in presets:"))
			for _, name := range preset.Names() {
				p, _ := preset.Get(name)
				fmt.Printf("  %s  %s\n",
					valueStyle.Render(fmt.Sprintf("%-14s", name)),
					labelStyle.Render(p.Description),
				)
			}
		},
	}
}
