package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/nothinglefttolearn/nltl-animate/internal/style"
)

// addRenderFlags registers the flags every rendering command shares.
func addRenderFlags(cmd *cobra.Command, o *renderOptions) {
	f := cmd.Flags()
	f.StringVar(&o.PresetName, "preset", o.PresetName,
		"visual preset (industrial, clean, dark, or custom from --config)")
	f.Var(bpmFlag{o}, "bpm", `track BPM for beat-synced animations, or "auto" to detect it (0 = time-based defaults)`)
	f.BoolVar(&o.Preview, "preview", o.Preview, "render a short low-quality preview")
	f.BoolVar(&o.Still, "still", o.Still, "disable spatial motion (zoom, jitter) — keeps brightness pulsing")
	f.BoolVarP(&o.Verbose, "verbose", "v", o.Verbose, "show raw ffmpeg output during render")
	f.StringVarP(&o.OutputDir, "output-dir", "o", o.OutputDir, "output directory (default: same as audio file)")
	f.StringVarP(&o.ConfigPath, "config", "c", o.ConfigPath, "YAML config file with custom presets")
}

// bpmFlag is --bpm: a whole number, or "auto" to detect it from the audio.
type bpmFlag struct{ o *renderOptions }

func (f bpmFlag) String() string {
	if f.o == nil { // pflag calls String on a zero value for the default
		return "0"
	}
	if f.o.AutoBPM {
		return "auto"
	}
	return strconv.Itoa(f.o.BPM)
}

func (f bpmFlag) Set(s string) error {
	if s == "auto" {
		f.o.BPM, f.o.AutoBPM = 0, true
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf(`want a whole number or "auto", got %q`, s)
	}
	f.o.BPM, f.o.AutoBPM = n, false
	return nil
}

func (bpmFlag) Type() string { return "bpm" }

// styleCmd builds a subcommand that renders one style. The shared flags are
// registered on it; extra registers the style's own flags.
func styleCmd(s clipStyle, use, short, long string, args cobra.PositionalArgs, extra func(*cobra.Command, *renderOptions)) *cobra.Command {
	opts := defaultRenderOptions()
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args:  args,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			opts.Style = s
			opts.AudioPath, opts.ImagePaths = args[0], args[1:]
			return run(opts)
		},
	}
	addRenderFlags(cmd, &opts)
	if extra != nil {
		extra(cmd, &opts)
	}
	return cmd
}

// addSwitchFlags registers the flags for styles that switch between several
// images over time (single and grid).
func addSwitchFlags(cmd *cobra.Command, o *renderOptions) {
	f := cmd.Flags()
	f.IntVar(&o.Hold, "hold", o.Hold, "with several images: frames to show each image for (default: 1 beat with --bpm, else 15)")
	f.Float64Var(&o.HoldBeats, "hold-beats", o.HoldBeats, "with several images: beats to show each image for (needs --bpm)")
	f.Uint64Var(&o.Seed, "seed", o.Seed, "with several images: random seed (0 = new random sequence)")
}

func gridCmd() *cobra.Command {
	return styleCmd(styleGrid,
		"grid <audio> <image> [image...]",
		"Tile the image(s) N×N, every tile moving in sync",
		`Tiles the image into an N×N grid. The preset's wobble is applied to each
tile identically; its grade (vignette, grain, brightness pulse) is applied once
over the whole frame. Requires ffmpeg 5.1 or newer.

With several images, the tiles switch between them at random (never the same
image twice in a row), every beat with --bpm or as set by --hold/--hold-beats:

  --tiles same    every tile shows the same image, switching together
  --tiles mixed   each tile shows a different image and switches on its own
                  sequence (all tiles switch at the same moments)

Every run prints its --seed so a clip can be reproduced exactly.`,
		cobra.MinimumNArgs(2),
		func(cmd *cobra.Command, o *renderOptions) {
			cmd.Flags().IntVarP(&o.GridN, "n", "n", o.GridN, "tiles per side (1–12)")
			cmd.Flags().StringVar(&o.Tiles, "tiles", o.Tiles, `with several images: "same" image in every tile, or "mixed"`)
			addSwitchFlags(cmd, o)
		},
	)
}

func trainCmd() *cobra.Command {
	d := style.DefaultTrainOptions()
	return styleCmd(styleTrain,
		"train <audio> <image> [image...]",
		"Copies of the image(s) fly past on the left, like watching a train go by",
		`Fakes a 3D scene: copies of the image travel along a track beside the
viewer, coming out of the distance and passing on the left. Transparent areas
of the image (e.g. a PNG logo) show the copies behind. Distant copies fade
into the background colour.

Give several images and the cars cycle through them in order, so a different
image passes at every step. Images keep their own shapes; their inner edges
line up along the track.

With --bpm, one copy passes every --beats beats; otherwise --speed copies
pass per second. --vanish moves the vanishing point to the right to fill more
of the frame. The preset's wobble and grade are applied over the result.`,
		cobra.MinimumNArgs(2),
		func(cmd *cobra.Command, o *renderOptions) {
			f := cmd.Flags()
			f.IntVar(&o.Train.Copies, "copies", d.Copies, "copies on the track at once")
			f.Float64Var(&o.Train.Spacing, "spacing", d.Spacing, "depth gap between copies, in copy heights")
			f.Float64Var(&o.Train.Speed, "speed", d.Speed, "copies passing per second (without --bpm)")
			f.Float64Var(&o.Train.Beats, "beats", d.Beats, "beats per passing copy (with --bpm)")
			f.Float64Var(&o.Train.Offset, "offset", d.Offset, "gap between the track and the viewer, in widths of the widest image")
			f.Float64Var(&o.Train.FOV, "fov", d.FOV, "horizontal field of view in degrees")
			f.Float64Var(&o.Train.Vanish, "vanish", d.Vanish, "vanishing point position across the frame (0.1–1)")
			f.Float64Var(&o.Train.Fog, "fog", d.Fog, "how much distant copies fade into the background (0–1)")
			f.StringVar(&o.TrainBG, "bg", o.TrainBG, "background colour (#rgb or #rrggbb)")
		},
	)
}
