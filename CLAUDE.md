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

`cmd/nltl-clip/main.go` (Cobra CLI) resolves a `Profile` (output dimensions, `internal/profile`) and a `Preset` (visual parameters, `internal/preset`, optionally overridden via `--config` YAML through `internal/config`), then calls `Preset.BuildFilter()` to generate a single ffmpeg `-vf` filter-graph string (zoompan/jitter/brightness expressions, BPM-synced or time-based) and hands it to `internal/ffmpeg.Render`, which shells out to ffmpeg directly — ffmpeg itself is the visual engine here, not just an encoder.

Zero-args invocation drops into `internal/interactive` (a `huh`-based TUI form) instead of requiring flags.

Presets and profiles both follow the same shape: a flat struct, a small hardcoded built-in map, `Get(name) (T, bool)`, and a sorted `Names()`. Custom presets loaded via `--config` are checked first and fall back to the built-in map by name.
