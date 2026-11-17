# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this directory. See the [repo-root CLAUDE.md](../CLAUDE.md) for cross-cutting conventions (in particular the documentation/description language rule) that also apply here.

## Commands

Tooling (Go, golangci-lint, govulncheck) is pinned in `mise.toml`; run `mise install` first.

```bash
mise run build        # build ./nltl-animate
mise run test         # go test -race with coverage (race detector needs a C compiler)
mise run lint         # golangci-lint (config in .golangci.yml; includes go vet)
mise run fmt          # gofmt + goimports
mise run ci           # everything CI runs: fmt:check, tidy:check, mod:verify, lint, test, vuln, build
./nltl-animate --preset industrial --bpm 138 --preview demo.wav cover.jpg
```

CI (`.github/workflows/ci.yml`) just runs `mise run ci`, plus a cross-compile matrix.

## Architecture

`cmd/nltl-animate` (Cobra CLI) resolves a `Preset` (visual parameters, `internal/preset`, optionally from `--config` YAML via `internal/config`) and a clip style, then hands `internal/ffmpeg.Render` a video input plus a filtergraph. ffmpeg itself is the visual engine, not just an encoder. `main.go` has the root command, `styles.go` the style subcommands and shared flags, and `render.go` the options, validation and per-style wiring (`renderOptions.videoInput`).

A preset's filter is two halves: `MotionFilter` (scale/crop/zoompan wobble, per frame) and `GradeFilter` (blur, brightness pulse, eq, vignette, grain). `BuildFilter` is motion followed by grade.

Every style takes one or more images. `ffmpeg.Render` has two video input kinds:
- a still image looped with `-loop 1` (`single` and `grid` with one image); `grid` is a pure filtergraph (`style.GridFilter`: motion at tile size → split → xstack → grade);
- a raw RGBA frame stream on stdin from an `ffmpeg.FrameSource`, rendered in Go in `internal/style`: `Cycle` (single with several images, or grid `--tiles same` at tile size under `GridFilter`) picks a pre-cropped image per frame; `Mosaic` (grid `--tiles mixed`) composes the whole grid with a separate sequence per tile, and `style.MosaicFilter` crops each tile back out to give it the same motion; `Train` projects camera-facing billboards onto a perspective track, renders frames in parallel, writes them in order. The preset's filter runs over these frames too.

`--no-audio` makes a silent clip of exactly `renderOptions.loopFrames()` frames (`ffmpeg.Options.NumFrames`, rendered with `-frames:v`) that loops seamlessly: one pass of a shuffled image order (`style.LoopSequence`, `MosaicLoopSequence`), whole image cycles of the train (`TrainOptions.LoopFrames` nudges the speed to fit), or 3 s for a single image. The preset's wobble is not matched up at the loop point.

`--bpm auto` decodes the audio to mono PCM (`ffmpeg.DecodeMono`) and estimates a whole-number tempo in `internal/tempo` (onset envelope → autocorrelation → beat-grid fit).

`internal/frames` decodes and cover-crops images; `internal/video` holds the shared frame rate and preview length.

Zero-args invocation drops into `internal/interactive` (a `huh`-based TUI form) instead of requiring flags.

Output is always square, `video.Size` (1080) on each side; resizing for particular players is deliberately out of scope. Presets are a flat struct, a small hardcoded built-in map, `Get(name) (Preset, bool)`, and a sorted `Names()`. Custom presets loaded via `--config` are checked first and fall back to the built-in map by name.

`mise run test:integration` renders a short clip of each style with the real ffmpeg (CI does this too); unit tests use fake `ffmpeg`/`ffprobe` shell scripts on PATH.
