package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nothinglefttolearn/nltl-animate/internal/grade"
	"github.com/nothinglefttolearn/nltl-animate/internal/layout"
	"github.com/nothinglefttolearn/nltl-animate/internal/motion"
	"github.com/nothinglefttolearn/nltl-animate/internal/outdir"
)

// addRenderFlags registers the flags every rendering command shares.
func addRenderFlags(cmd *cobra.Command, o *renderOptions) {
	f := cmd.Flags()
	f.StringVar(&o.GradeName, "grade", o.GradeName,
		"how the picture looks: "+strings.Join(grade.Names(), ", ")+", or custom from --config")
	f.StringVar(&o.MotionName, "motion", o.MotionName,
		"how the picture moves with the beat: "+strings.Join(motion.Names(), ", ")+", or custom from --config")
	f.Var(bpmFlag{o}, "bpm", `track BPM for the motion and image switching, "auto", or "disable" to ignore the beat (default: detected from the audio; 120 with --no-audio)`)
	f.BoolVar(&o.Preview, "preview", o.Preview, "render a short low-quality preview")
	f.BoolVar(&o.NoAudio, "no-audio", o.NoAudio,
		"no audio file: every argument is an image, and the silent clip loops seamlessly")
	f.BoolVarP(&o.Verbose, "verbose", "v", o.Verbose, "show raw ffmpeg output during render")
	f.StringVarP(&o.OutputDir, "output-dir", "o", o.OutputDir, "output directory (default: $"+outdir.EnvVar+" if set, else same as audio file, or first image with --no-audio)")
	f.BoolVar(&o.Overwrite, "overwrite", o.Overwrite, "replace an existing output file without asking")
	f.StringVarP(&o.ConfigPath, "config", "c", o.ConfigPath, "YAML config file with custom grades and motions")
	f.Uint64Var(&o.Seed, "seed", o.Seed, "with several images or a shade: random seed for the image order and shades (0 = new each run)")
}

// bpmFlag is --bpm: a whole number, "auto" to detect it from the audio, or
// "disable" for no tempo: the motion runs at its own pace and nothing waits
// for the beat.
type bpmFlag struct{ o *renderOptions }

func (f bpmFlag) String() string {
	if f.o == nil { // pflag calls String on a zero value for the default
		return "0"
	}
	switch {
	case f.o.AutoBPM:
		return "auto"
	case f.o.NoBPM:
		return "disable"
	}
	return strconv.Itoa(f.o.BPM)
}

func (f bpmFlag) Set(s string) error {
	switch s {
	case "auto":
		f.o.BPM, f.o.AutoBPM, f.o.NoBPM = 0, true, false
		return nil
	case "disable":
		f.o.BPM, f.o.AutoBPM, f.o.NoBPM = 0, false, true
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf(`want a whole number, "auto" or "disable", got %q`, s)
	}
	f.o.BPM, f.o.AutoBPM, f.o.NoBPM = n, false, false
	return nil
}

func (bpmFlag) Type() string { return "bpm" }

// setArgs fills the audio and image paths from the positional arguments:
// audio then images, or only images with --no-audio.
func (o *renderOptions) setArgs(args []string) {
	if o.NoAudio {
		o.AudioPath, o.ImagePaths = "", args
		return
	}
	o.AudioPath, o.ImagePaths = args[0], args[1:]
}

// minArgs is the fewest positional arguments a render needs.
func (o *renderOptions) minArgs() int {
	if o.NoAudio {
		return 1
	}
	return 2
}

// layoutCmd builds a subcommand that renders one layout. The shared flags are
// registered on it; extra registers the layout's own flags.
func layoutCmd(l layoutName, use, short, long string, extra func(*cobra.Command, *renderOptions)) *cobra.Command {
	opts := defaultRenderOptions()
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < opts.minArgs() {
				return fmt.Errorf("expected audio and image(s), or image(s) with --no-audio; got %d arg(s)", len(args))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			opts.Layout = l
			opts.setArgs(args)
			return run(opts)
		},
	}
	addRenderFlags(cmd, &opts)
	if extra != nil {
		extra(cmd, &opts)
	}
	return cmd
}

// addSwitchFlags registers the flags for layouts that switch between several
// images, or shades, over time (single and grid).
func addSwitchFlags(cmd *cobra.Command, o *renderOptions) {
	f := cmd.Flags()
	f.IntVar(&o.Hold, "hold", o.Hold, "with several images or a shade: frames to show each image for (default: 1 beat, or 15 without a tempo)")
	f.Float64Var(&o.HoldBeats, "hold-beats", o.HoldBeats, "with several images or a shade: beats to show each image for (needs --bpm with --no-audio)")
}

func gridCmd() *cobra.Command {
	return layoutCmd(layoutGrid,
		"grid <audio> <image> [image...]",
		"Tile the image(s) N×N, every tile moving in sync",
		`Tiles the image into an N×N grid. The motion's breath and shake are applied
to each tile identically; its pulse and the grade (vignette, grain, ...) are
applied once over the whole frame. Requires ffmpeg 5.1 or newer.

With several images, the tiles switch between them at random (never the same
image twice in a row), every beat with --bpm or as set by --hold/--hold-beats:

  --tiles same    every tile shows the same image, switching together
  --tiles mixed   each tile shows a different image and switches on its own
                  sequence (all tiles switch at the same moments)

With a motion that has a shade (e.g. --motion shifting), every tile gets its
own lighter or darker shade at every switch, even with a single image.

Every run prints its --seed so a clip can be reproduced exactly.

With --no-audio, the silent clip loops seamlessly: one switch per image (the
mixed tiles also differ from the first step at the loop point), or 3 seconds
with a single image.`,
		func(cmd *cobra.Command, o *renderOptions) {
			cmd.Flags().IntVarP(&o.GridN, "n", "n", o.GridN, "tiles per side (1–12)")
			cmd.Flags().StringVar(&o.Tiles, "tiles", o.Tiles, `with several images: "same" image in every tile, or "mixed"`)
			addSwitchFlags(cmd, o)
		},
	)
}

func trainCmd() *cobra.Command {
	d := layout.DefaultTrainOptions()
	return layoutCmd(layoutTrain,
		"train <audio> <image> [image...]",
		"Copies of the image(s) fly past on the left, like watching a train go by",
		`Fakes a 3D scene: copies of the image travel along a track beside the
viewer, coming out of the distance and passing on the left. Transparent areas
of the image (e.g. a PNG logo) show the copies behind. Distant copies fade
into the background color.

Give several images and the cars go through them in a random order: each
image once, then again in a new order, never the same image twice in a row
(--seed fixes the order). Images keep their own shapes; their inner edges
line up along the track.

One copy passes every --beats beats (at the detected tempo unless --bpm is
given); with --bpm disable, or a silent loop without --bpm, it passes --speed
copies per second instead. --vanish moves the vanishing point to the right to fill more
of the frame. The motion and grade are applied over the result; a motion
with a shade (e.g. --motion shifting) makes every car a little lighter or
darker.

With --no-audio, the silent clip loops seamlessly: it lasts while one copy of
each image passes (at least 3 seconds with a single image), with the speed
nudged slightly so the loop is a whole number of frames.`,
		func(cmd *cobra.Command, o *renderOptions) {
			f := cmd.Flags()
			f.IntVar(&o.Train.Copies, "copies", d.Copies, "copies on the track at once")
			f.Float64Var(&o.Train.Spacing, "spacing", d.Spacing, "depth gap between copies, in copy heights")
			f.Float64Var(&o.Train.Speed, "speed", d.Speed, "copies passing per second (without a tempo: --bpm disable, or --no-audio without --bpm)")
			f.Float64Var(&o.Train.Beats, "beats", d.Beats, "beats per passing copy (with audio or --bpm)")
			f.Float64Var(&o.Train.Offset, "offset", d.Offset, "gap between the track and the viewer, in widths of the widest image")
			f.Float64Var(&o.Train.FOV, "fov", d.FOV, "horizontal field of view in degrees")
			f.Float64Var(&o.Train.Vanish, "vanish", d.Vanish, "vanishing point position across the frame (0.1–1)")
			f.Float64Var(&o.Train.Fog, "fog", d.Fog, "how much distant copies fade into the background (0–1)")
			f.StringVar(&o.TrainBG, "bg", o.TrainBG, "background color (#rgb or #rrggbb)")
		},
	)
}
