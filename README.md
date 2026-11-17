# nltl-animate

A CLI tool that animates still images to music: it takes a track and one or more images and renders a video in which the images move, switch and travel in time with the beat. Built around ffmpeg.

The output is always a 1080×1080 square video. Fitting it to a particular player or aspect ratio (letterboxing, pillarboxing, cropping) is left to a separate step.

## Requirements

- [Go](https://go.dev) 1.27.4+ (or [mise](https://mise.jdx.dev): `mise install`)
- [ffmpeg](https://ffmpeg.org) with ffprobe in your PATH (5.1 or newer for the `grid` style)

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
nltl-animate       <audio> <image> [image...] [flags]   # single (default style)
nltl-animate grid  <audio> <image> [image...] [flags]
nltl-animate train <audio> <image> [image...] [flags]
nltl-animate                                            # interactive: asks for everything
```

```bash
# Preview 10 seconds before committing to a full render
nltl-animate --preset industrial --bpm 138 --preview demo.wav cover.jpg

# Full render
nltl-animate --preset industrial --bpm 138 demo.wav cover.jpg

# Detect the tempo instead of passing it
nltl-animate --preset dark --bpm auto demo.wav cover.jpg

# Use a custom preset from a config file
nltl-animate --config nltl.yaml --preset my-preset --bpm 138 demo.wav cover.jpg
```

Output files are saved alongside the audio file (or in `--output-dir`), named after the track and the style: `demo_single.mp4`, `demo_cycle.mp4` (single with several images), `demo_grid4.mp4`, `demo_grid3_mixed.mp4`, `demo_train.mp4`, with `_preview` added for previews.

## Styles

Every style gets the preset's motion (zoom breathing, jitter) and grade (blur, brightness pulse, contrast, saturation, vignette, grain), so `--preset`, `--bpm` and `--still` work everywhere. Run `nltl-animate <style> --help` for each style's flags.

Every style takes one or more images. Single and grid switch between several images over time; train puts a different image in each car.

### Switching between images (single and grid)

With several images, the clip jumps to a random image (never the same one twice in a row) every beat with `--bpm`, or every half second without. Change the rate with `--hold` (frames) or `--hold-beats` (beats, needs `--bpm`):

```bash
nltl-animate --bpm 140 demo.wav frames/*.png                       # new image every beat
nltl-animate --hold 1 demo.wav frames/*.png                         # every frame: jittery animation
nltl-animate --bpm 140 --hold-beats 0.25 demo.wav frames/*.png      # every 16th note
```

The sequence is random but reproducible: each render prints its `--seed`, and passing the same seed gives the same clip. Images are cover-cropped to the output (or tile) size; each one uses about `width × height × 4` bytes of memory (8 MB at 1080×1920).

### single

The image gently moving. The default style: just pass `audio image [image...]`.

### grid

The image tiled N×N, every tile moving in sync. The vignette and grain are applied once over the whole frame.

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

A fake 3D scene: copies of the image come out of the distance and pass on your left, like standing beside a track watching a train go by. Use a PNG with transparency (a logo, a cut-out) so the copies behind show through. Distant copies fade into the background colour.

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
| `--bg` | `#000000` | background colour |

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--preset` | `industrial` | Visual style |
| `--bpm` | `0` | Track BPM for beat-synced animation, or `auto` to detect it (0 = time-based) |
| `--preview` | `false` | Render 10s at low quality to check the look |
| `--output-dir`, `-o` | same as audio | Where to write the output file |
| `--still` | `false` | Disable zoom and jitter (brightness pulse stays) |
| `--verbose`, `-v` | `false` | Show ffmpeg's output while rendering |
| `--config`, `-c` | — | YAML file with custom presets |
| `--version` | — | Print the version and exit |

## Presets

| Name | Description |
|------|-------------|
| `industrial` | Desaturated, grainy, heavy vignette |
| `clean` | Full colour, minimal grain, subtle movement |
| `dark` | Deep blacks, extreme grain, maximum vignette |

Run `nltl-animate presets` to list them.

## Custom Presets

Copy `nltl.yaml.example` to `nltl.yaml` and edit the values. Each preset starts from another preset (`extends`, default `industrial`) and only overrides the fields you list:

```yaml
presets:
  - name: my-preset
    description: My custom look
    extends: dark        # built-in, or a custom preset defined earlier in the file
    contrast: 1.4
    grain_strength: 30
```

| Field | Range | Notes |
|-------|-------|-------|
| `zoom_base` | 1–10 | base zoom level |
| `zoom_amp` | ≥ 0 | zoom oscillation amplitude |
| `zoom_beats` | > 0 | beats per zoom cycle |
| `jitter_amp` | ≥ 0 | pixel micro-jitter |
| `blur_sigma` | 0–1024 | gaussian blur (0 = sharp) |
| `contrast` | -1000–1000 | 1 = unchanged |
| `saturation` | 0–3 | 0 = monochrome, 1 = unchanged |
| `brightness_amp` | 0–1 | brightness pulse amplitude |
| `brightness_beats` | > 0 | beats per brightness cycle |
| `vignette_fraction` | 0–0.5 | vignette angle as a fraction of π |
| `grain_strength` | 0–100 | noise intensity |

The config is checked when it's loaded. Unknown keys (typos), values out of range, duplicate names, and presets without a `name` are all reported together.

`--preset` looks in the config first, then the built-ins, so a custom preset can replace a built-in of the same name.

## BPM Animations

When `--bpm` is set, zoom, jitter, and brightness oscillations are tied to the track's beat grid rather than fixed time intervals. The `zoom_beats` and `brightness_beats` fields in a preset control how many beats each animation cycle spans.

At 138 BPM with `zoom_beats: 8`, the zoom breath completes one cycle every ~3.5 seconds — one slow inhale per 8 beats.

`--bpm auto` detects the tempo from the audio (the first 5 minutes) and prints it in the summary and in the "run this again" command, so you can pin it once you know it. In the interactive form, type `auto` in the BPM field.

```bash
nltl-animate train --bpm auto demo.wav logo.png
```

Detection is built in (no extra tools beyond ffmpeg) and gives a whole-number BPM, which suits tracks made at a fixed tempo. It is most reliable on music with a steady kick. Like any tempo detector, it can land on half or double the tempo you'd count (e.g. 87 for drum & bass at 174). The visuals still hit the beat then, just at half or twice the rate. Pass `--bpm 174` to override. Tracks with no clear beat give an error rather than a guess.
