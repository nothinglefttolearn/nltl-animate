package layout

import (
	"image"
	"io"
	"testing"
)

func benchTrain(b *testing.B) *Train {
	b.Helper()
	tr, err := NewTrain([]image.Image{ring(1024)}, 1080, 1920, DefaultTrainOptions())
	if err != nil {
		b.Fatal(err)
	}
	return tr
}

func BenchmarkTrainRenderFrame(b *testing.B) {
	tr := benchTrain(b)
	dst := image.NewRGBA(image.Rect(0, 0, 1080, 1920))
	i := 0
	for b.Loop() {
		tr.RenderFrame(dst, i)
		i++
	}
}

func BenchmarkTrainWriteFrames(b *testing.B) {
	tr := benchTrain(b)
	for b.Loop() {
		if err := tr.WriteFrames(io.Discard, 60); err != nil {
			b.Fatal(err)
		}
	}
}
