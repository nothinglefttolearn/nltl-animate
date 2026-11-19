# Split presets into grade and motion

A "preset" bundled eleven raw ffmpeg parameters covering two unrelated things: how the picture moves and how it looks. Neither a preset's name nor its values told you what a render would look like. We replaced it with two independent named choices, a **motion** (breath, shake, pulse) and a **grade** (contrast, saturation, softness, vignette, grain), each with five dials in units you can see. Strengths are percentages of the frame or image, rates are in beats, and a grade with every dial at 0 leaves the image untouched. The word "preset" is retired, and `--preset` and `--still` were removed outright rather than deprecated. `still` is now just a built-in motion.

## Considered Options

- **Keep one preset, redesign its parameters.** Rejected: you still couldn't pair a look with a different movement, and `--still` would remain a special override.
- **A third concept, "pulse", chosen separately.** Rejected: a whole flag for one parameter. The pulse lives in motion because motion is defined as everything that follows the beat, and grade as everything fixed.
- **Per-dial CLI flags (`--grain 40`).** Rejected for now: they would add ten flags, and a render's grade and motion names would no longer fully describe it. Tuning happens in custom grades and motions in the config file, and `grades`/`motions` list every dial value. Per-dial flags can be added later without breaking anything.

## Consequences

- Configs with a `presets:` key are rejected with an error explaining the new `grades:`/`motions:` format. There is no automatic conversion.
- The zoom crop-in (`zoom_base`) is no longer a setting. It is derived from breath and shake so that image edges never show.
