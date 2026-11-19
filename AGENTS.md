# AGENTS.md

Guidance for agents and contributors working in this repository.

- [CONTEXT.md](CONTEXT.md) is the glossary: layout, grade, motion and their parts. Use its terms.
- [docs/adr/](docs/adr/) records decisions that would otherwise look surprising (e.g. [why presets were split into grade and motion](docs/adr/0001-split-presets-into-grade-and-motion.md)).

## Writing

- American English: "color", "center", "normalize", "canceled". It matches CSS and most programming languages.
- User-facing text (README, `--help`, error messages, descriptions of built-in grades and motions) describes what you see and hear, not ffmpeg parameter names.
- Use the terms in `CONTEXT.md` and never the ones it lists under _Avoid_. In particular, there is no "style" (it is a **layout**) and no "preset" (it is a **grade** or a **motion**).

## Commands

Tooling (Go, golangci-lint, govulncheck) is pinned in `mise.toml`; run `mise install` first.

```bash
mise run build        # build ./nltl-animate
mise run test         # go test -race with coverage (race detector needs a C compiler)
mise run lint         # golangci-lint (config in .golangci.yml; includes go vet)
mise run fmt          # gofmt + goimports
mise run ci           # everything CI runs: fmt:check, tidy:check, mod:verify, lint, test, vuln, build
./nltl-animate --grade muted --motion steady --bpm 138 --preview demo.wav cover.jpg
```

CI (`.github/workflows/ci.yml`) just runs `mise run ci`, plus a cross-compile matrix.

## Architecture

`cmd/nltl-animate` (Cobra CLI) resolves a layout, a `grade.Grade` and a `motion.Motion` (each by name: custom ones from `--config` YAML via `internal/config` first, then the built-ins), then hands `internal/ffmpeg.Render` a video input plus a filtergraph. ffmpeg itself is the visual engine, not just an encoder. `main.go` has the root command and the `grades`/`motions` listings, `layouts.go` the layout subcommands and shared flags, and `render.go` the options, validation and per-layout wiring (`renderOptions.videoInput`).

The treatment over a layout is the motion's `MoveFilter` (scale/crop/zoompan: breath and shake, per frame, at tile size in a grid) followed by the grade's `Filter`, which takes the motion's `PulseExpr` as the `eq` brightness so the pulse shares the grade's pass. `layout.Filter` joins the two for a whole frame. Grade dials are percentages with 0 = untouched; motion strengths are percentages of the frame and rates are in beats (at `motion.AssumedBPM` without `--bpm`). The zoompan crop-in is derived from breath + shake so image edges never show. Each package translates its dials into ffmpeg values; nothing outside them should know the ffmpeg units. The motion's tint is the exception to "over a layout": layouts apply it in Go while building frames, one cast per appearance of an image from `motion.Tinter` (see [ADR 0002](docs/adr/0002-tint-lives-in-motion.md)), so a tinted single image or grid becomes a frame stream (`Cycle`, `NewSameMosaic` with `MosaicFilter`) instead of a looped still.

Grades and motions are each a flat struct, a small hardcoded built-in map, `Get(name)`, a sorted `Names()`, `Validate()` and `Summary()` (dial values in plain words). In the config, `grades:` and `motions:` entries `extends` a built-in or earlier custom one of the same kind; an old `presets:` key is rejected with a migration message.

Every layout takes one or more images. `ffmpeg.Render` has two video input kinds:
- a still image looped with `-loop 1` (`single` and `grid` with one image and no tint); `grid` is a pure filtergraph (`layout.GridFilter`: move at tile size → split → xstack → pulse and grade);
- a raw RGBA frame stream on stdin from an `ffmpeg.FrameSource`, rendered in Go in `internal/layout`: `Cycle` (single with several images, or grid `--tiles same` at tile size under `GridFilter`) picks a pre-cropped image per frame; `Mosaic` (grid `--tiles mixed`) composes the whole grid with a separate sequence per tile, and `layout.MosaicFilter` crops each tile back out to give it the same movement; `Train` projects camera-facing billboards onto a perspective track, renders frames in parallel, writes them in order. The motion and grade run over these frames too.

`--no-audio` makes a silent clip of exactly `renderOptions.loopFrames()` frames (`ffmpeg.Options.NumFrames`, rendered with `-frames:v`) that loops seamlessly: one pass of a shuffled image order (`layout.LoopSequence`, `MosaicLoopSequence`), whole image cycles of the train (`TrainOptions.LoopFrames` nudges the speed to fit), or 3 s for a single image. The motion is not matched up at the loop point.

`--bpm auto` decodes the audio to mono PCM (`ffmpeg.DecodeMono`) and estimates a whole-number tempo in `internal/tempo` (onset envelope → autocorrelation → beat-grid fit).

`internal/frames` decodes and cover-crops images; `internal/video` holds the shared frame rate and preview length.

Zero-args invocation drops into `internal/interactive` (a `huh`-based TUI form) instead of requiring flags.

Output is always square, `video.Size` (1080) on each side; resizing for particular players is deliberately out of scope. Output file names keep `cycle` for single with several images (`nltl_animate_cycle_…`).

`mise run test:integration` renders a short clip of each layout with the real ffmpeg (CI does this too); unit tests use fake `ffmpeg`/`ffprobe` shell scripts on PATH.
