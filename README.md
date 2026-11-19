# nltl-animate

A CLI tool that animates still images to music: it takes a track and one or more images and renders a video in which the images move, switch and travel in time with the beat. Built around ffmpeg.

The output is always a 1080×1080 square video. Fitting it to a particular player or aspect ratio (letterboxing, pillarboxing, cropping) is left to a separate step.

Every clip is made of three independent choices:

- a **layout**: how the images are arranged in the frame (`single`, `grid`, `train`);
- a **grade**: how the picture looks (contrast, saturation, softness, vignette, grain);
- a **motion**: how the picture moves with the beat (breath, shake, pulse).

See [CONTEXT.md](CONTEXT.md) for the full vocabulary.

## Requirements

- [Go](https://go.dev) 1.27.4+ (or [mise](https://mise.jdx.dev): `mise install`)
- [ffmpeg](https://ffmpeg.org) with ffprobe in your PATH (5.1 or newer for the `grid` layout)

## Install

```bash
go install github.com/nothinglefttolearn/nltl-animate/cmd/nltl-animate@latest
```

Or build locally:

```bash
git clone https://github.com/nothinglefttolearn/nltl-animate
cd nltl-animate
go build -o nltl-animate ./cmd/nltl-animate/
```

## Usage

```
nltl-animate       <audio> <image> [image...] [flags]   # single (default layout)
nltl-animate grid  <audio> <image> [image...] [flags]
nltl-animate train <audio> <image> [image...] [flags]
nltl-animate --no-audio <image> [image...] [flags]      # silent loop, any layout
nltl-animate                                            # interactive: asks for everything
nltl-animate grades                                     # list grades and their dial values
nltl-animate motions                                    # list motions and their dial values
```

```bash
# Preview 10 seconds before committing to a full render
nltl-animate --grade muted --motion steady --bpm 138 --preview demo.wav cover.jpg

# Full render
nltl-animate --grade muted --motion steady --bpm 138 demo.wav cover.jpg

# Detect the tempo instead of passing it
nltl-animate --grade dark --motion restless --bpm auto demo.wav cover.jpg

# A photograph that should not move at all
nltl-animate --grade clean --motion still demo.wav photo.jpg

# Use a custom grade from a config file
nltl-animate --config nltl.yaml --grade my-grade --bpm 138 demo.wav cover.jpg
```

Output files are saved alongside the audio file (or the first image with `--no-audio`, or in `--output-dir`). Each name gives the layout, whether the clip has audio, and the time of the render, so repeated renders never replace each other:

```
nltl_animate_single_audio_20261117-215301.mp4
nltl_animate_cycle_audio_preview_20261117-215301.mp4        # single with several images, --preview
nltl_animate_grid3_mixed_loop_noaudio_20261117-215301.mp4   # grid --tiles mixed --no-audio
```

If the file already exists (two renders in the same second), you are asked before it is replaced. Without a terminal to ask on, the render stops instead, unless you pass `--overwrite`.

### Silent loops (`--no-audio`)

With `--no-audio` there is no audio file: every argument is an image, and the output is a silent clip that loops seamlessly when played on repeat. It works with every layout, saved next to the first image.

- **Several images (single, grid):** the clip is one pass through the images. They are shuffled once (by `--seed`) and each is shown once, for `--hold`/`--hold-beats`, so playing it on repeat shows the same sequence over and over, with the loop point falling exactly on an image switch. With `--tiles mixed` there is one step per image, and every tile also changes image at the loop point.
- **One image (single, grid):** 3 seconds, long enough for the motion's breath and shake to read as movement.
- **train:** as long as it takes one car per image to pass (at least 3 seconds with one image). The speed is nudged slightly so that is a whole number of frames, and the last frame leads straight back into the first.

The motion's breath, shake and pulse are not matched up at the loop point; they are jumpy anyway. `--bpm <number>` still sets the tempo of the motion and switching; `--bpm auto` needs audio. `--preview` stops at 10 seconds, which cuts long loops short.

```bash
nltl-animate --no-audio cover.jpg                                    # 3s loop
nltl-animate --no-audio --bpm 140 frames/*.png                       # one pass, an image per beat
nltl-animate grid --no-audio --n 3 --tiles mixed --hold 10 frames/*.png
```

## Layouts

Every layout gets the grade and the motion, so `--grade`, `--motion` and `--bpm` work everywhere. Run `nltl-animate <layout> --help` for each layout's flags.

Every layout takes one or more images. Single and grid switch between several images over time; train puts a different image in each car.

### Switching between images (single and grid)

With several images, the clip jumps to a random image (never the same one twice in a row) every beat with `--bpm`, or every half second without. Change the rate with `--hold` (frames) or `--hold-beats` (beats, needs `--bpm`):

```bash
nltl-animate --bpm 140 demo.wav frames/*.png                       # new image every beat
nltl-animate --hold 1 demo.wav frames/*.png                         # every frame: jittery animation
nltl-animate --bpm 140 --hold-beats 0.25 demo.wav frames/*.png      # every 16th note
```

The sequence is random but reproducible: each render prints its `--seed`, and passing the same seed gives the same clip. Images are cover-cropped to the output (or tile) size; each one uses about `width × height × 4` bytes of memory (about 4.7 MB at 1080×1080).

### single

The image filling the frame. The default layout: just pass `audio image [image...]`.

### grid

The image tiled N×N, every tile moving in sync: each tile gets the motion's breath and shake, while the pulse, vignette and grain are applied once over the whole frame.

```bash
nltl-animate grid --n 4 demo.wav cover.jpg
```

With several images, `--tiles` picks how they are spread over the tiles:

- `--tiles same` (default): every tile shows the same image and they all switch together. The grid repeats one sequence N×N times.
- `--tiles mixed`: each tile shows a different image and follows its own random sequence. All tiles switch at the same moments, every tile moves to a new image at each switch, and no two tiles show the same image at once as long as there are at least as many images as tiles (otherwise the images are spread as evenly as possible).

```bash
nltl-animate grid --n 3 --bpm 138 demo.wav frames/*.png                  # same image everywhere
nltl-animate grid --n 3 --tiles mixed --bpm 138 demo.wav frames/*.png    # a different image per tile
```

### train

A fake 3D scene: copies of the image come out of the distance and pass on your left, like standing beside a track watching a train go by. Use a PNG with transparency (a logo, a cut-out) so the copies behind show through. Distant copies fade into the background color.

Pass several images and the cars cycle through them in order, so a different image passes at every step. Each image keeps its own shape; their inner edges line up along the track.

```bash
nltl-animate train --bpm 138 demo.wav logo.png                 # one copy passes per beat
nltl-animate train --bpm 138 --beats 0.5 --vanish 0.7 --bg '#101820' demo.wav logo.png
nltl-animate train --bpm 138 demo.wav a.png b.png c.png        # a, b, c, a, b, c, ... down the line
```

| Flag | Default | Description |
|------|---------|-------------|
| `--copies` | `24` | copies on the track at once |
| `--spacing` | `1` | depth gap between copies, in copy heights (bigger = more separated) |
| `--speed` | `1.5` | copies passing per second, without `--bpm` |
| `--beats` | `1` | beats per passing copy, with `--bpm` |
| `--vanish` | `0.5` | where the track vanishes across the frame; `0.6`–`0.75` fills more of it |
| `--offset` | `0.25` | gap between the track and you, in widths of the widest image |
| `--fov` | `60` | field of view in degrees (wider = stronger perspective) |
| `--fog` | `0.6` | how much distant copies fade into the background, 0–1 |
| `--bg` | `#000000` | background color |

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--grade` | `muted` | How the picture looks: a built-in or custom grade |
| `--motion` | `steady` | How the picture moves with the beat: a built-in or custom motion |
| `--bpm` | `0` | Track BPM for the motion and image switching, or `auto` to detect it (0 = assume 120) |
| `--preview` | `false` | Render 10s at low quality to check the look |
| `--no-audio` | `false` | No audio file: arguments are all images, output is a silent seamless loop |
| `--output-dir`, `-o` | same as audio (or first image) | Where to write the output file |
| `--overwrite` | `false` | Replace an existing output file without asking |
| `--verbose`, `-v` | `false` | Show ffmpeg's output while rendering |
| `--config`, `-c` | — | YAML file with custom grades and motions |
| `--version` | — | Print the version and exit |

## Grades

A grade is the fixed color and texture of the picture. Nothing in it follows the beat. Every dial is a percentage, and **0 leaves the image untouched**.

| Dial | Range | Meaning |
|------|-------|---------|
| `contrast` | -100 to 100 | % change: `+15` is punchier, `-20` flatter |
| `saturation` | -100 to 100 | % change: `-100` is black and white |
| `softness` | 0 to 100 | blur: 0 is sharp, 100 heavily smeared |
| `vignette` | 0 to 100 | how dark the corners get: 0 is none |
| `grain` | 0 to 100 | film grain: 0 is clean, 100 very heavy |

| Grade | contrast | saturation | softness | vignette | grain | |
|-------|---------:|-----------:|---------:|---------:|------:|---|
| `clean` | +5 | 0 | 3 | 20 | 8 | Full color, light vignette, a trace of grain |
| `muted` | +15 | -30 | 6 | 33 | 25 | Pulled-back color, visible grain, medium vignette (default) |
| `dark` | +30 | -60 | 8 | 50 | 40 | Little color, hard contrast, heavy grain and vignette |

## Motions

A motion is everything that moves in time with the beat. Strengths are percentages of the frame; rates are in beats. Without `--bpm` the beats run at 120 BPM.

| Dial | Range | Meaning |
|------|-------|---------|
| `breath` | 0 to 50 | how far the frame zooms in and out, % |
| `breath_beats` | > 0 | beats per breath |
| `shake` | 0 to 10 | how far the frame drifts and jitters, % (at a fixed rate, every 2 beats) |
| `pulse` | 0 to 100 | how much the brightness swells, % |
| `pulse_beats` | > 0 | beats per pulse |

| Motion | breath | breath_beats | shake | pulse | pulse_beats | |
|--------|-------:|-------------:|------:|------:|------------:|---|
| `still` | 0 | — | 0 | 0 | — | No movement at all (suggested for photographs) |
| `gentle` | 0.5 | 16 | 0.1 | 1 | 8 | Slow, barely-there drift |
| `steady` | 1 | 8 | 0.2 | 2 | 4 | Noticeable breathing that keeps time (default) |
| `restless` | 1.5 | 4 | 0.3 | 4 | 2 | Fast, deep breathing and a strong pulse |

The frame is always zoomed in just enough that the breath and shake never show the image's edges.

At 138 BPM with `breath_beats: 8`, the breath completes one cycle every ~3.5 seconds: one slow inhale per 8 beats.

## Custom grades and motions

Run `nltl-animate grades` and `nltl-animate motions` (add `--config nltl.yaml` to include yours) to see every dial value.

Copy `nltl.yaml.example` to `nltl.yaml` and edit the values. Each entry starts from another of the same kind (`extends`, default `muted` for grades and `steady` for motions) and only overrides the dials you list:

```yaml
grades:
  - name: my-grade
    description: My custom look
    extends: dark        # built-in, or a custom grade defined earlier in the file
    contrast: 40
    grain: 30

motions:
  - name: slow-breath
    extends: gentle
    breath: 2
    breath_beats: 16
```

The config is checked when it's loaded. Unknown keys (typos), values out of range, duplicate names, and entries without a `name` are all reported together. A grade can only extend a grade, and a motion only a motion.

`--grade` and `--motion` look in the config first, then the built-ins, so a custom one can replace a built-in of the same name.

## Tempo

`--bpm auto` detects the tempo from the audio (the first 5 minutes) and prints it in the summary and in the "run this again" command, so you can pin it once you know it. In the interactive form, type `auto` in the BPM field.

```bash
nltl-animate train --bpm auto demo.wav logo.png
```

Detection is built in (no extra tools beyond ffmpeg) and gives a whole-number BPM, which suits tracks made at a fixed tempo. It is most reliable on music with a steady kick. Like any tempo detector, it can land on half or double the tempo you'd count (e.g. 87 for drum & bass at 174). The visuals still hit the beat then, just at half or twice the rate. Pass `--bpm 174` to override. Tracks with no clear beat give an error rather than a guess.
