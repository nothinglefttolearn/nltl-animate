package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/nothinglefttolearn/nltl-clip/internal/interactive"
	"github.com/nothinglefttolearn/nltl-clip/internal/preset"
	"github.com/nothinglefttolearn/nltl-clip/internal/profile"
)

var (
	labelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	valueStyle  = lipgloss.NewStyle().Bold(true)
	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
)

// version can be set at build time:
//
//	go build -ldflags "-X main.version=v1.2.3" ./cmd/nltl-clip
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
		Use:   "nltl-clip [audio] [image]",
		Short: "Generate social media video clips from audio and images",
		Long: `Generate social media video clips from audio and images.

With an audio file and an image, renders the single-image style. Other
styles are subcommands (cycle, grid, train). With no arguments, an
interactive form asks for everything.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || len(args) == 2 {
				return nil
			}
			return fmt.Errorf("expected 0 args (interactive) or 2 args (audio image), got %d", len(args))
		},
		Version: buildVersion(),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Args and flags parsed fine, so any error from here on is a
			// runtime failure (bad config, ffmpeg) and usage text is noise.
			cmd.SilenceUsage = true

			if len(args) == 2 {
				opts.Style = styleSingle
				opts.AudioPath, opts.ImagePaths = args[0], args[1:]
				return run(opts)
			}
			return runInteractive(opts)
		},
	}
	addRenderFlags(root, &opts)

	root.AddCommand(
		cycleCmd(), gridCmd(), trainCmd(),
		presetsCmd(), profilesCmd(),
	)
	return root
}

func runInteractive(opts renderOptions) error {
	cfg, err := loadConfig(opts.ConfigPath)
	if err != nil {
		return err
	}
	choices, err := interactive.Run(interactive.Defaults{
		ProfileName:   opts.ProfileName,
		PresetName:    opts.PresetName,
		BPM:           opts.BPM,
		Still:         opts.Still,
		GridN:         opts.GridN,
		CustomPresets: cfg.Presets,
	})
	if err != nil {
		return err
	}
	opts.Style = clipStyle(choices.Style)
	opts.AudioPath = choices.AudioPath
	opts.ImagePaths = choices.ImagePaths
	opts.ProfileName = choices.ProfileName
	opts.PresetName = choices.PresetName
	opts.BPM = choices.BPM
	opts.Still = choices.Still
	opts.GridN = choices.GridN

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

func profilesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "profiles",
		Short: "List available output profiles",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(valueStyle.Render("Output profiles:"))
			for _, name := range profile.Names() {
				p, _ := profile.Get(name)
				fmt.Printf("  %s  %s\n",
					valueStyle.Render(fmt.Sprintf("%-22s", name)),
					labelStyle.Render(fmt.Sprintf("%dx%d", p.Width, p.Height)),
				)
			}
		},
	}
}
