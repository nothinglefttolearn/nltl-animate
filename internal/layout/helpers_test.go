package layout

import (
	"image"
	"image/color"
	"math"
)

// ring returns an n x n opaque color ring with transparent center and
// corners, standing in for a logo with an alpha channel.
func ring(n int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	c := float64(n) / 2
	for y := range n {
		for x := range n {
			d := math.Hypot(float64(x)-c+0.5, float64(y)-c+0.5) / c
			if d > 0.55 && d < 0.95 {
				img.SetRGBA(x, y, color.RGBA{R: 255, G: 64, B: 128, A: 255})
			}
		}
	}
	return img
}

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRGBA(img, c)
	return img
}
