# Tint lives in motion, though layouts apply it per image

**Tint** gives each appearance of an image its own color cast: each tile in a grid, each new car on the train. Because it is per image, layouts apply it while building frames, not as a filter over the finished frame like the other motion parts. We still made it a motion dial (`tint` in a motion, plus a built-in `shifting`), because ADR 0001 places everything that follows the beat in motion and everything fixed in the grade. The casts change exactly when the layout shows a new image, which is on the beat. Motion is now "everything that changes in time with the beat, over the whole frame or per image".

## Considered Options

- **A whole-frame color flicker in the ffmpeg filtergraph** (per-channel `eq` gamma, re-rolled every N beats). Cheapest, and it works on every layout unchanged. Rejected: one cast over the whole frame can't give each train car or grid tile its own color, which was the point.
- **A layout flag (`--tint 10`) next to `--hold-beats`.** Close to the timing that drives it, but a layout "says nothing about how the result looks", and ADR 0001 kept dial values out of CLI flags.
- **A grade dial.** It is color, but a grade is fixed by definition, and tint changes with every switch.
- **Tinted copies made by the separate image-generation tool.** More files, fixed colors, and no knowledge of the tempo.

## Consequences

- With tint on, a one-image single or grid can no longer be a looped still image with a pure filtergraph. It becomes a sequence of tinted copies, switching every hold.
- In a grid, every tile is its own appearance, even with `--tiles same`.
- Casts are balanced (brightness stays the pulse's job), always at full strength, and spaced well apart around the color wheel so neighbors clearly differ. They follow `--seed` and repeat exactly at a `--no-audio` loop point.
