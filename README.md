# nltl-clip

A CLI tool for generating animated video clips from a music file and a still image. Built around ffmpeg with BPM-synced visuals and output profiles for social media platforms.

## Requirements

- [Go](https://go.dev) 1.27.4+ (or [mise](https://mise.jdx.dev): `mise install`)
- [ffmpeg](https://ffmpeg.org) with ffprobe in your PATH

## Install

```bash
go install github.com/nothinglefttolearn/nltl-clip/cmd/nltl-clip@latest
```

Or build locally:

```bash
git clone https://github.com/nothinglefttolearn/nltl-clip
cd nltl-clip
go build -o nltl-clip ./cmd/nltl-clip/
```

## Usage

```
nltl-clip [audio] [image] [flags]
```

```bash
# Preview 10 seconds before committing to a full render
nltl-clip --preset industrial --bpm 138 --preview demo.wav cover.jpg

# Full render for Instagram feed
nltl-clip --preset industrial --bpm 138 demo.wav cover.jpg

# YouTube version
nltl-clip --profile youtube --preset dark --bpm 138 demo.wav cover.jpg

# Use a custom preset from a config file
nltl-clip --config nltl.yaml --preset my-preset --bpm 138 demo.wav cover.jpg
```

Output files are saved alongside the audio file, named `{trackname}_{profile}.mp4`.

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--profile`, `-p` | `instagram-square` | Output dimensions |
| `--preset` | `industrial` | Visual style |
| `--bpm` | `0` | Track BPM for beat-synced animation (0 = time-based) |
| `--preview` | `false` | Render 10s at low quality to check the look |
| `--output-dir`, `-o` | same as audio | Where to write the output file |
| `--config`, `-c` | — | YAML file with custom presets |
| `--version` | — | Print the version and exit |

## Profiles

| Name | Resolution |
|------|------------|
| `instagram-square` | 1080×1080 |
| `instagram-story` | 1080×1920 |
| `youtube` | 1920×1080 |

Run `nltl-clip profiles` to list them.

## Presets

| Name | Description |
|------|-------------|
| `industrial` | Desaturated, grainy, heavy vignette |
| `clean` | Full colour, minimal grain, subtle movement |
| `dark` | Deep blacks, extreme grain, maximum vignette |

Run `nltl-clip presets` to list them.

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
