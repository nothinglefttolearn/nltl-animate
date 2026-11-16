package main

import (
	"github.com/spf13/cobra"

	"github.com/nothinglefttolearn/nltl-clip/internal/style"
)

// addRenderFlags registers the flags every rendering command shares.
func addRenderFlags(cmd *cobra.Command, o *renderOptions) {
	f := cmd.Flags()
	f.StringVarP(&o.ProfileName, "profile", "p", o.ProfileName,
		"output profile (instagram-square, instagram-story, youtube)")
	f.StringVar(&o.PresetName, "preset", o.PresetName,
		"visual preset (industrial, clean, dark, or custom from --config)")
	f.IntVar(&o.BPM, "bpm", o.BPM, "track BPM for beat-synced animations (0 = time-based defaults)")
	f.BoolVar(&o.Preview, "preview", o.Preview, "render a short low-quality preview")
	f.BoolVar(&o.Still, "still", o.Still, "disable spatial motion (zoom, jitter) — keeps brightness pulsing")
	f.BoolVarP(&o.Verbose, "verbose", "v", o.Verbose, "show raw ffmpeg output during render")
	f.StringVarP(&o.OutputDir, "output-dir", "o", o.OutputDir, "output directory (default: same as audio file)")
	f.StringVarP(&o.ConfigPath, "config", "c", o.ConfigPath, "YAML config file with custom presets")
}

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

func cycleCmd() *cobra.Command {
	return styleCmd(styleCycle,
		"cycle <audio> <image> <image> [image...]",
		"Jump between images at random, with the preset's motion on top",
		`Shows a random one of the images on every frame (never the same one twice
in a row), with the preset's wobble and grade applied over the result. The
images loosely act as animation frames.

Use --hold to keep each image for several frames, or --hold-beats with --bpm
to switch on the beat grid. Every run prints its --seed so a clip can be
reproduced exactly.`,
		cobra.MinimumNArgs(3),
		func(cmd *cobra.Command, o *renderOptions) {
			f := cmd.Flags()
			f.IntVar(&o.Hold, "hold", o.Hold, "frames to show each image for")
			f.Float64Var(&o.HoldBeats, "hold-beats", o.HoldBeats, "beats to show each image for (needs --bpm)")
			f.Uint64Var(&o.Seed, "seed", o.Seed, "random seed (0 = new random sequence)")
		},
	)
}

func gridCmd() *cobra.Command {
	return styleCmd(styleGrid,
		"grid <audio> <image>",
		"Tile the image N×N, every tile moving in sync",
		`Duplicates the image into an N×N grid. The preset's wobble is applied to each
tile identically; its grade (vignette, grain, brightness pulse) is applied once
over the whole frame. Requires ffmpeg 5.1 or newer.`,
		cobra.ExactArgs(2),
		func(cmd *cobra.Command, o *renderOptions) {
			cmd.Flags().IntVarP(&o.GridN, "n", "n", o.GridN, "tiles per side (1–12)")
		},
	)
}

func trainCmd() *cobra.Command {
	d := style.DefaultTrainOptions()
	return styleCmd(styleTrain,
		"train <audio> <image>",
		"Copies of the image fly past on the left, like watching a train go by",
		`Fakes a 3D scene: copies of the image travel along a track beside the
viewer, coming out of the distance and passing on the left. Transparent areas
of the image (e.g. a PNG logo) show the copies behind. Distant copies fade
into the background colour.

With --bpm, one copy passes every --beats beats; otherwise --speed copies
pass per second. --vanish moves the vanishing point to the right to fill more
of the frame. The preset's wobble and grade are applied over the result.`,
		cobra.ExactArgs(2),
		func(cmd *cobra.Command, o *renderOptions) {
			f := cmd.Flags()
			f.IntVar(&o.Train.Copies, "copies", d.Copies, "copies on the track at once")
			f.Float64Var(&o.Train.Spacing, "spacing", d.Spacing, "depth gap between copies, in copy heights")
			f.Float64Var(&o.Train.Speed, "speed", d.Speed, "copies passing per second (without --bpm)")
			f.Float64Var(&o.Train.Beats, "beats", d.Beats, "beats per passing copy (with --bpm)")
			f.Float64Var(&o.Train.Offset, "offset", d.Offset, "gap between the track and the viewer, in copy widths")
			f.Float64Var(&o.Train.FOV, "fov", d.FOV, "horizontal field of view in degrees")
			f.Float64Var(&o.Train.Vanish, "vanish", d.Vanish, "vanishing point position across the frame (0.1–1)")
			f.Float64Var(&o.Train.Fog, "fog", d.Fog, "how much distant copies fade into the background (0–1)")
			f.StringVar(&o.TrainBG, "bg", o.TrainBG, "background colour (#rgb or #rrggbb)")
		},
	)
}
