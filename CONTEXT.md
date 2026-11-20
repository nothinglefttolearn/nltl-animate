# nltl-animate

Animates still images to music: a track and one or more images become a square video in which the images move, switch and travel in time with the beat.

## Language

### Arrangement

**Layout**:
How the images are arranged in the frame and how they get there: one image, a tiled grid, or a perspective track. Says nothing about how the result looks.
_Avoid_: Style, mode, scene

**Single**:
The layout showing one image filling the frame. The default layout.

**Grid**:
The layout tiling the frame N×N, every tile moving in sync.

**Train**:
The layout in which copies of the images travel along a perspective track and pass the viewer on the left, like a train going by.

### Treatment

**Motion**:
A named setting for everything that changes in time with the beat, over the whole frame or per image: breath, shake, pulse and shade. Chosen independently of the grade; built-in or custom.
_Avoid_: Preset, wobble, animation

**Breath**:
The part of a motion where the frame slowly zooms in and out, once every so many beats. Its strength is how far it zooms, as a percentage of the frame.
_Avoid_: Zoom amplitude

**Shake**:
The part of a motion where the frame drifts and jitters around its center at a fixed rate. Its strength is how far it moves, as a percentage of the frame.
_Avoid_: Jitter, wobble

**Pulse**:
The part of a motion where the whole frame swells brighter and darker, once every so many beats. Its strength is the brightness change, as a percentage.
_Avoid_: Brightness amplitude, flash

**Shade**:
The part of a motion where each appearance of an image (a new image in a tile, a new car on the train) is shown a little lighter or a little darker than the original, and stays that way while it is on screen. Its strength is the largest brightness change, as a percentage. Unlike the pulse, it does not swell with the beat and is not the same across the frame.
_Avoid_: Tint, flicker, color variation, exposure

**Still**:
The built-in motion with no breath, shake, pulse or shade.

**Custom**:
A grade or motion defined by the user in a config file rather than built in. Otherwise the same kind of thing as a built-in one.
_Avoid_: Custom preset

**Grade**:
A named setting for the fixed color and texture treatment applied over a layout: contrast, saturation, softness, vignette, grain. Nothing in a grade follows the beat. Chosen independently of the motion; built-in or custom.
_Avoid_: Preset, look, style, filter
