# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this directory. See the [repo-root CLAUDE.md](../CLAUDE.md) for cross-cutting conventions (in particular the documentation/description language rule) that also apply here.

## Commands

Tooling (Go, golangci-lint, govulncheck) is pinned in `mise.toml`; run `mise install` first.

```bash
mise run build        # build ./nltl-clip
mise run test         # go test -race with coverage (race detector needs a C compiler)
mise run lint         # golangci-lint (config in .golangci.yml; includes go vet)
mise run fmt          # gofmt + goimports
mise run ci           # everything CI runs: fmt:check, tidy:check, mod:verify, lint, test, vuln, build
./nltl-clip --preset industrial --bpm 138 --preview demo.wav cover.jpg
```

CI (`.github/workflows/ci.yml`) just runs `mise run ci`, plus a cross-compile matrix.

## Architecture

`cmd/nltl-clip` (Cobra CLI) resolves a `Profile` (output dimensions, `internal/profile`), a `Preset` (visual parameters, `internal/preset`, optionally from `--config` YAML via `internal/config`) and a clip style, then hands `internal/ffmpeg.Render` a video input plus a filtergraph. ffmpeg itself is the visual engine, not just an encoder. `main.go` has the root command, `styles.go` the style subcommands and shared flags, and `render.go` the options, validation and per-style wiring (`renderOptions.videoInput`).

A preset's filter is two halves: `MotionFilter` (scale/crop/zoompan wobble, per frame) and `GradeFilter` (blur, brightness pulse, eq, vignette, grain). `BuildFilter` is motion followed by grade.

`ffmpeg.Render` has two video input kinds:
- a still image looped with `-loop 1` (styles `single`, `grid`); `grid` is a pure filtergraph (`style.GridFilter`: motion at tile size → split → xstack → grade);
- a raw RGBA frame stream on stdin from an `ffmpeg.FrameSource` (styles `cycle`, `train`), rendered in Go in `internal/style` (`Cycle` picks a pre-cropped image per frame; `Train` projects camera-facing billboards onto a perspective track, renders frames in parallel, writes them in order). The preset's full filter runs over these frames too.

`internal/frames` decodes and cover-crops images; `internal/video` holds the shared frame rate and preview length.

Zero-args invocation drops into `internal/interactive` (a `huh`-based TUI form) instead of requiring flags.

Presets and profiles both follow the same shape: a flat struct, a small hardcoded built-in map, `Get(name) (T, bool)`, and a sorted `Names()`. Custom presets loaded via `--config` are checked first and fall back to the built-in map by name.

`mise run test:integration` renders a short clip of each style with the real ffmpeg (CI does this too); unit tests use fake `ffmpeg`/`ffprobe` shell scripts on PATH.
