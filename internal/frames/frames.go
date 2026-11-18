// Package frames loads images and prepares RGBA frames that are streamed to
// ffmpeg as raw video.
package frames

import (
	"fmt"
	"image"
	_ "image/gif" // register decoders for image.Decode
	_ "image/jpeg"
	_ "image/png"
	"os"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// Load decodes a PNG, JPEG, GIF (first frame) or WebP image.
func Load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() // read-only: a close error cannot lose data
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", path, err)
	}
	return img, nil
}

// ToRGBA converts img to a premultiplied *image.RGBA whose bounds start at 0,0.
func ToRGBA(img image.Image) *image.RGBA {
	b := img.Bounds()
	if rgba, ok := img.(*image.RGBA); ok && b.Min == (image.Point{}) {
		return rgba
	}
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

// Cover scales img so it fills w x h and crops the overflow evenly from both
// sides, like CSS object-fit: cover (and ffmpeg's
// force_original_aspect_ratio=increase followed by crop).
func Cover(img image.Image, w, h int) *image.RGBA {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()

	// Source rectangle with the target aspect ratio, centered.
	src := b
	if sw*h > sh*w { // source is wider: trim left/right
		cw := sh * w / h
		src.Min.X += (sw - cw) / 2
		src.Max.X = src.Min.X + cw
	} else { // source is taller: trim top/bottom
		ch := sw * h / w
		src.Min.Y += (sh - ch) / 2
		src.Max.Y = src.Min.Y + ch
	}

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, src, draw.Src, nil)
	return dst
}
