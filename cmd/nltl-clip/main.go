package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/nothinglefttolearn/nltl-clip/internal/config"
	"github.com/nothinglefttolearn/nltl-clip/internal/ffmpeg"
	"github.com/nothinglefttolearn/nltl-clip/internal/interactive"
	"github.com/nothinglefttolearn/nltl-clip/internal/preset"
	"github.com/nothinglefttolearn/nltl-clip/internal/profile"
)

var (
	labelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	valueStyle  = lipgloss.NewStyle().Bold(true)
	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
)

func label(s string) string { return labelStyle.Render(fmt.Sprintf("%-10s", s)) }

// renderOptions is everything needed to render one clip. It is filled from
// flags, and in interactive mode the TUI overrides the fields it asks about.
type renderOptions struct {
	AudioPath   string
	ImagePath   string
	ProfileName string
	PresetName  string
	BPM         int
	Preview     bool
	Still       bool
	Verbose     bool
	OutputDir   string
	ConfigPath  string
}

func main() {
	var opts renderOptions

	root := &cobra.Command{
		Use:   "nltl-clip [audio] [image]",
		Short: "Generate social media video clips from audio and a still image",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || len(args) == 2 {
				return nil
			}
			return fmt.Errorf("expected 0 args (interactive) or 2 args (audio image), got %d", len(args))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig(opts.ConfigPath)
			if err != nil {
				return err
			}

			if len(args) == 2 {
				opts.AudioPath, opts.ImagePath = args[0], args[1]
				return renderClip(opts, cfg)
			}

			choices, err := interactive.Run(interactive.Defaults{
				ProfileName:   opts.ProfileName,
				PresetName:    opts.PresetName,
				BPM:           opts.BPM,
				Still:         opts.Still,
				CustomPresets: cfg.Presets,
			})
			if err != nil {
				return err
			}
			opts.AudioPath = choices.AudioPath
			opts.ImagePath = choices.ImagePath
			opts.ProfileName = choices.ProfileName
			opts.PresetName = choices.PresetName
			opts.BPM = choices.BPM
			opts.Still = choices.Still

			fmt.Println()
			if err := renderClip(opts, cfg); err != nil {
				return err
			}
			fmt.Printf("\nTo run this again without prompts:\n  %s\n", opts.headlessCommand())
			return nil
		},
	}

	root.Flags().StringVarP(&opts.ProfileName, "profile", "p", "instagram-square",
		"output profile (instagram-square, instagram-story, youtube)")
	root.Flags().StringVar(&opts.PresetName, "preset", "industrial",
		"visual preset (industrial, clean, dark, or custom from --config)")
	root.Flags().IntVar(&opts.BPM, "bpm", 0, "track BPM for beat-synced animations (0 = time-based defaults)")
	root.Flags().BoolVar(&opts.Preview, "preview", false, "render a 10s low-quality preview")
	root.Flags().BoolVar(&opts.Still, "still", false, "disable spatial motion (zoom, jitter) — keeps brightness pulsing")
	root.Flags().BoolVarP(&opts.Verbose, "verbose", "v", false, "show raw ffmpeg output during render")
	root.Flags().StringVarP(&opts.OutputDir, "output-dir", "o", "", "output directory (default: same as audio file)")
	root.Flags().StringVarP(&opts.ConfigPath, "config", "c", "", "YAML config file with custom presets")

	root.AddCommand(presetsCmd(), profilesCmd())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
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

// headlessCommand returns the flag-driven invocation equivalent to opts.
func (o renderOptions) headlessCommand() string {
	parts := []string{
		"nltl-clip",
		"--profile", o.ProfileName,
		"--preset", o.PresetName,
	}
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
	parts = append(parts, o.AudioPath, o.ImagePath)
	return strings.Join(parts, " ")
}

func renderClip(opts renderOptions, cfg *config.Config) error {
	prof, ok := profile.Get(opts.ProfileName)
	if !ok {
		return fmt.Errorf("unknown profile %q — try: %s", opts.ProfileName, strings.Join(profile.Names(), ", "))
	}

	p, err := resolvePreset(opts.PresetName, cfg)
	if err != nil {
		return err
	}

	if opts.Still {
		p.ZoomAmp = 0
		p.JitterAmp = 0
		p.ZoomBase = 1.0
	}

	duration, err := ffmpeg.GetDuration(opts.AudioPath)
	if err != nil {
		return fmt.Errorf("reading audio duration: %w", err)
	}

	base := strings.TrimSuffix(filepath.Base(opts.AudioPath), filepath.Ext(opts.AudioPath))
	outDir := opts.OutputDir
	if outDir == "" {
		outDir = filepath.Dir(opts.AudioPath)
	}

	suffix := prof.Suffix
	if opts.Preview {
		suffix += "_preview"
	}
	outputPath := filepath.Join(outDir, fmt.Sprintf("%s_%s.mp4", base, suffix))

	filter := p.BuildFilter(prof.Width, prof.Height, opts.BPM)

	fmt.Println()
	fmt.Println(label("Profile") + valueStyle.Render(fmt.Sprintf("%s (%dx%d)", prof.Name, prof.Width, prof.Height)))
	fmt.Println(label("Preset") + valueStyle.Render(fmt.Sprintf("%s — %s", p.Name, p.Description)))
	if opts.BPM > 0 {
		fmt.Println(label("BPM") + valueStyle.Render(strconv.Itoa(opts.BPM)))
	}
	if opts.Preview {
		fmt.Println(label("Preview") + valueStyle.Render("10s low-quality render"))
	}
	if opts.Still {
		fmt.Println(label("Still") + valueStyle.Render("yes (spatial motion disabled)"))
	}
	fmt.Println(label("Output") + accentStyle.Render(outputPath))
	fmt.Println()

	return ffmpeg.Render(ffmpeg.Options{
		ImagePath:  opts.ImagePath,
		AudioPath:  opts.AudioPath,
		OutputPath: outputPath,
		Filter:     filter,
		Duration:   duration,
		Preview:    opts.Preview,
		Verbose:    opts.Verbose,
	})
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
