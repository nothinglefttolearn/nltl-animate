# Shade lives in motion, though layouts apply it per image

**Shade** makes each appearance of an image a little lighter or darker: each tile in a grid at each switch, each new car on the train. Because it is per image, layouts apply it while building frames, not as a filter over the finished frame like the other motion parts. We still made it a motion dial (`shade` in a motion, plus a built-in `shifting`), because ADR 0001 places everything that follows the beat in motion and everything fixed in the grade. The shades change exactly when the layout shows a new image, which is on the beat. Motion is now "everything that changes in time with the beat, over the whole frame or per image".

It started out as **tint**: a random color cast per appearance, stepping far around the color wheel each time. In use the hue jumping from image to image looked uncontrolled, so before it was released it became a brightness change only, picked at random within the strength. The image's own colors are never touched.

## Considered Options

- **A whole-frame brightness flicker in the ffmpeg filtergraph** (re-rolled every N beats). Cheapest, and it works on every layout unchanged. Rejected: one shade over the whole frame can't give each train car or grid tile its own, which was the point, and it would be hard to tell apart from the pulse.
- **A layout flag (`--shade 10`) next to `--hold-beats`.** Close to the timing that drives it, but a layout "says nothing about how the result looks", and ADR 0001 kept dial values out of CLI flags.
- **A grade dial.** A grade is fixed by definition, and shade changes with every switch.
- **Shaded copies made by the separate image-generation tool.** More files, fixed shades, and no knowledge of the tempo.

## Consequences

- With shade on, a one-image single or grid can no longer be a looped still image with a pure filtergraph. It becomes a sequence of shaded copies, switching every hold.
- In a grid, every tile is its own appearance, even with `--tiles same`.
- Shade and pulse both change brightness, but differently: the pulse swells the whole frame with the beat; a shade is fixed for as long as one image is on screen and differs from image to image.
- Shades are plain random, so two neighbors can come out close. They follow `--seed` and repeat exactly at a `--no-audio` loop point.
