package layout

import (
	"bytes"
	"image"
	"image/color"
	"slices"
	"testing"
)

func TestSequenceDeterministicAndNoRepeats(t *testing.T) {
	a := Sequence(5, 1000, 1, 42)
	if !slices.Equal(a, Sequence(5, 1000, 1, 42)) {
		t.Fatal("same seed gave different sequences")
	}
	if slices.Equal(a, Sequence(5, 1000, 1, 43)) {
		t.Error("different seeds gave identical sequences")
	}
	seen := map[int]bool{}
	for i, v := range a {
		if v < 0 || v >= 5 {
			t.Fatalf("index %d out of range", v)
		}
		if i > 0 && v == a[i-1] {
			t.Fatalf("frame %d repeats image %d", i, v)
		}
		seen[v] = true
	}
	if len(seen) != 5 {
		t.Errorf("only %d of 5 images used", len(seen))
	}
}

func TestSequenceTwoImagesAlternate(t *testing.T) {
	s := Sequence(2, 50, 1, 7)
	for i := 1; i < len(s); i++ {
		if s[i] == s[i-1] {
			t.Fatalf("2 images must alternate, got %v", s)
		}
	}
}

func TestSequenceHold(t *testing.T) {
	s := Sequence(4, 30, 4, 1)
	for i := range s {
		block := i / 4
		if s[i] != s[block*4] {
			t.Fatalf("frame %d should hold image %d, got %d", i, s[block*4], s[i])
		}
		if i%4 == 0 && i > 0 && s[i] == s[i-1] {
			t.Fatalf("frame %d: new block repeats previous image", i)
		}
	}
}

func TestHoldFromBeats(t *testing.T) {
	cases := []struct {
		beats float64
		bpm   int
		want  int
	}{
		{1, 120, 15},   // 0.5s
		{0.25, 120, 4}, // 3.75 frames -> 4
		{4, 138, 52},   // 1.739s
		{0.01, 200, 1}, // never below 1
	}
	for _, c := range cases {
		if got := HoldFromBeats(c.beats, c.bpm); got != c.want {
			t.Errorf("HoldFromBeats(%v, %d) = %d, want %d", c.beats, c.bpm, got, c.want)
		}
	}
}

func TestCycleWriteFrames(t *testing.T) {
	red := solid(4, 2, color.RGBA{R: 255, A: 255})
	blue := solid(4, 2, color.RGBA{B: 255, A: 255})
	c, err := NewCycle([]*image.RGBA{red, blue}, 1, 9)
	if err != nil {
		t.Fatal(err)
	}
	if w, h := c.Size(); w != 4 || h != 2 {
		t.Fatalf("Size = %dx%d", w, h)
	}

	var buf bytes.Buffer
	if err := c.WriteFrames(&buf, 6); err != nil {
		t.Fatal(err)
	}
	const frameLen = 4 * 2 * 4
	if buf.Len() != 6*frameLen {
		t.Fatalf("wrote %d bytes, want %d", buf.Len(), 6*frameLen)
	}
	for i, idx := range Sequence(2, 6, 1, 9) {
		px := buf.Bytes()[i*frameLen : i*frameLen+4]
		wantRed := idx == 0
		if (px[0] == 255) != wantRed || (px[2] == 255) == wantRed {
			t.Errorf("frame %d pixel %v, want image %d", i, px, idx)
		}
	}
}

func TestNewCycleErrors(t *testing.T) {
	a := solid(2, 2, color.RGBA{A: 255})
	if _, err := NewCycle(nil, 1, 0); err == nil {
		t.Error("expected error for no images")
	}
	if _, err := NewCycle([]*image.RGBA{a, a}, 0, 0); err == nil {
		t.Error("expected error for hold 0")
	}
	if _, err := NewCycle([]*image.RGBA{a, solid(3, 2, color.RGBA{A: 255})}, 1, 0); err == nil {
		t.Error("expected error for mismatched sizes")
	}
}

func TestLoopSequenceRepeatsEachImageOncePerPass(t *testing.T) {
	const nImages, hold = 5, 3
	pass := nImages * hold
	s := LoopSequence(nImages, 3*pass, hold, 9)
	if !slices.Equal(s, LoopSequence(nImages, 3*pass, hold, 9)) {
		t.Fatal("same seed gave different sequences")
	}
	if !slices.Equal(s[:pass], s[pass:2*pass]) || !slices.Equal(s[:pass], s[2*pass:]) {
		t.Fatalf("passes differ: %v", s)
	}
	seen := map[int]bool{}
	for i := range pass {
		if i%hold != 0 && s[i] != s[i-1] {
			t.Fatalf("frame %d switches mid-hold: %v", i, s)
		}
		seen[s[i]] = true
	}
	if len(seen) != nImages {
		t.Errorf("a pass shows %d of %d images: %v", len(seen), nImages, s[:pass])
	}
	if s[pass-1] == s[0] {
		t.Errorf("loop point repeats image %d", s[0])
	}
}

func TestCycleLoopWritesLoopSequence(t *testing.T) {
	imgs := []*image.RGBA{
		solid(2, 2, color.RGBA{R: 1, A: 255}),
		solid(2, 2, color.RGBA{R: 2, A: 255}),
		solid(2, 2, color.RGBA{R: 3, A: 255}),
	}
	c, err := NewCycle(imgs, 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	c.Loop = true
	var buf bytes.Buffer
	if err := c.WriteFrames(&buf, 12); err != nil {
		t.Fatal(err)
	}
	for i, idx := range LoopSequence(3, 12, 2, 4) {
		if got := int(buf.Bytes()[i*16]); got != idx+1 {
			t.Fatalf("frame %d shows image %d, want %d", i, got-1, idx)
		}
	}
}
