package tempo

import (
	"errors"
	"math"
	"math/cmplx"
	"math/rand/v2"
	"testing"
)

// pattern describes a synthetic drum loop.
type pattern struct {
	bpm      float64
	seconds  float64
	offset   float64 // time of the first beat
	kick     []float64
	hat      []float64 // positions within a 4-beat bar, in beats
	snare    []float64
	noise    float64 // background noise amplitude
	pad      bool    // add a sustained chord (energy with no onsets)
	introSec float64 // beatless intro (pad and noise only)
}

func fourOnTheFloor(bpm float64) pattern {
	return pattern{
		bpm: bpm, seconds: 40, offset: 0.37,
		kick:  []float64{0, 1, 2, 3},
		hat:   []float64{0.5, 1.5, 2.5, 3.5},
		snare: []float64{1, 3},
		noise: 0.02, pad: true,
	}
}

func synth(p pattern, seed uint64) []float32 {
	rng := rand.New(rand.NewPCG(seed, 1))
	n := int(p.seconds * SampleRate)
	out := make([]float64, n)
	beat := 60 / p.bpm
	add := func(at float64, f func(t float64) float64, length float64) {
		start := int(at * SampleRate)
		for i := 0; i < int(length*SampleRate) && start+i < n; i++ {
			if start+i >= 0 {
				out[start+i] += f(float64(i) / SampleRate)
			}
		}
	}
	kick := func(t float64) float64 {
		return 0.9 * math.Sin(2*math.Pi*(50*t+60*(1-math.Exp(-t*30))/30)) * math.Exp(-t*12)
	}
	noiseHit := func(amp, decay float64) func(float64) float64 {
		return func(t float64) float64 { return amp * (rng.Float64()*2 - 1) * math.Exp(-t*decay) }
	}
	start := p.offset + p.introSec
	for bar := start; bar < p.seconds; bar += 4 * beat {
		for _, b := range p.kick {
			add(bar+b*beat, kick, 0.3)
		}
		for _, b := range p.hat {
			add(bar+b*beat, noiseHit(0.25, 60), 0.08)
		}
		for _, b := range p.snare {
			add(bar+b*beat, noiseHit(0.5, 18), 0.2)
		}
	}
	for i := range out {
		t := float64(i) / SampleRate
		if p.pad {
			out[i] += 0.15 * (math.Sin(2*math.Pi*220*t) + math.Sin(2*math.Pi*277*t) + math.Sin(2*math.Pi*330*t))
		}
		out[i] += p.noise * (rng.Float64()*2 - 1)
	}
	f := make([]float32, n)
	for i, v := range out {
		f[i] = float32(v)
	}
	return f
}

func TestDetectFourOnTheFloor(t *testing.T) {
	for _, bpm := range []float64{90, 100, 120, 124, 128, 133, 138, 145, 150} {
		got, err := Detect(synth(fourOnTheFloor(bpm), uint64(bpm)))
		if err != nil {
			t.Fatalf("%v BPM: %v", bpm, err)
		}
		if got != int(bpm) {
			t.Errorf("%v BPM: detected %d", bpm, got)
		}
	}
}

// At the edges of the range the pattern is genuinely ambiguous between
// two metrical levels; either is acceptable, but nothing else.
func TestDetectEdgeTemposWithinAnOctave(t *testing.T) {
	for _, bpm := range []int{70, 174} {
		got, err := Detect(synth(fourOnTheFloor(float64(bpm)), uint64(bpm)))
		if err != nil {
			t.Fatalf("%d BPM: %v", bpm, err)
		}
		if got != bpm && got != 2*bpm && 2*got != bpm {
			t.Errorf("%d BPM: detected %d, want %d, %d or %d", bpm, got, bpm/2, bpm, 2*bpm)
		}
	}
}

func TestDetectKickOnly(t *testing.T) {
	for _, bpm := range []float64{100, 133, 160} {
		p := pattern{bpm: bpm, seconds: 30, kick: []float64{0, 1, 2, 3}, noise: 0.01}
		if got, err := Detect(synth(p, 3)); err != nil || got != int(bpm) {
			t.Errorf("%v BPM kick only: got %d, %v", bpm, got, err)
		}
	}
}

func TestDetectBreakbeat(t *testing.T) {
	// Kicks and snares off the four-to-the-floor grid, eighth-note hats.
	p := pattern{
		bpm: 96, seconds: 40, offset: 0.1,
		kick:  []float64{0, 0.75, 2.5},
		snare: []float64{1, 3},
		hat:   []float64{0, 0.5, 1, 1.5, 2, 2.5, 3, 3.5},
		noise: 0.02,
	}
	if got, err := Detect(synth(p, 9)); err != nil || got != 96 {
		t.Errorf("breakbeat at 96: got %d, %v", got, err)
	}
}

func TestDetectFractionalTempoRounds(t *testing.T) {
	if got, err := Detect(synth(fourOnTheFloor(127.8), 4)); err != nil || got != 128 {
		t.Errorf("127.8 BPM: got %d, %v", got, err)
	}
}

func TestDetectWithBeatlessIntro(t *testing.T) {
	p := fourOnTheFloor(140)
	p.seconds, p.introSec = 50, 12
	if got, err := Detect(synth(p, 5)); err != nil || got != 140 {
		t.Errorf("140 BPM after an intro: got %d, %v", got, err)
	}
}

func TestDetectNoBeat(t *testing.T) {
	silence := make([]float32, 20*SampleRate)
	if _, err := Detect(silence); !errors.Is(err, ErrNoBeat) {
		t.Errorf("silence: err = %v, want ErrNoBeat", err)
	}
	for name, p := range map[string]pattern{
		"drone": {bpm: 120, seconds: 20, pad: true, noise: 0.05},
		"noise": {bpm: 120, seconds: 20, noise: 0.3},
	} {
		if bpm, err := Detect(synth(p, 6)); !errors.Is(err, ErrNoBeat) {
			t.Errorf("%s: got %d, %v; want ErrNoBeat", name, bpm, err)
		}
	}
}

func TestDetectTooShort(t *testing.T) {
	if _, err := Detect(make([]float32, SampleRate)); err == nil {
		t.Error("expected an error for 1s of audio")
	}
}

func TestFFTMatchesDFT(t *testing.T) {
	x := make([]complex128, 16)
	for i := range x {
		x[i] = complex(math.Sin(float64(i)*0.7)+float64(i%3), float64(i%5)*0.1)
	}
	want := make([]complex128, len(x))
	for k := range want {
		for n, v := range x {
			want[k] += v * cmplx.Exp(complex(0, -2*math.Pi*float64(k*n)/float64(len(x))))
		}
	}
	fft(x)
	for k := range x {
		if cmplx.Abs(x[k]-want[k]) > 1e-9 {
			t.Fatalf("bin %d: got %v, want %v", k, x[k], want[k])
		}
	}
}
