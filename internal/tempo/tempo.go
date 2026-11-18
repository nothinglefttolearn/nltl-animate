// Package tempo estimates a track's tempo from mono PCM samples.
//
// The method is the usual onset-strength one: a spectral-flux onset envelope
// (how much new energy appears at each moment, which peaks on kicks, snares
// and other attacks), then
//
//  1. a coarse search over 60–200 BPM scoring each tempo by the envelope's
//     autocorrelation at one to four beat periods, weighted by a broad prior
//     around 120 BPM to settle half/double-tempo ties, and
//  2. a fine pass that tries whole BPM values around the coarse estimate and
//     keeps the one whose beat grid lines up best with the onsets across the
//     whole track. Small tempo errors accumulate along a long track, so this
//     is far more precise than the autocorrelation peak.
//
// It works best on music with a steady pulse (electronic, rock, pop). Like
// any detector it can land on half or double the felt tempo, e.g. 87 for
// drum & bass at 174; beat-synced visuals still line up with the music then,
// just at half or twice the rate.
package tempo

import (
	"errors"
	"math"
	"math/cmplx"
)

const (
	// SampleRate is the rate Detect expects. Onsets need no more than this.
	SampleRate = 11025

	MinBPM = 60
	MaxBPM = 200

	frameSize = 512 // FFT size (~46 ms)
	hop       = 128 // envelope step (~11.6 ms, ~86 Hz)

	// envRate is the onset envelope's sample rate.
	envRate = float64(SampleRate) / hop

	// MinSeconds is the shortest audio Detect accepts: enough for several
	// beats at the slowest tempo.
	MinSeconds = 8
)

// ErrNoBeat is returned when the audio has no usable rhythmic content
// (silence, or a drone with no attacks).
var ErrNoBeat = errors.New("no clear beat found")

// Detect returns the estimated tempo in whole BPM of mono samples at
// SampleRate.
func Detect(samples []float32) (int, error) {
	if len(samples) < MinSeconds*SampleRate {
		return 0, errors.New("audio too short to detect a tempo")
	}
	env := onsetEnvelope(samples)
	if env == nil {
		return 0, ErrNoBeat
	}
	bpm := fineTempo(env, coarseTempo(env))
	if !clearBeat(env, float64(bpm)) {
		return 0, ErrNoBeat
	}
	return bpm, nil
}

// minBeatContrast is how much stronger onsets must be on the best beat grid
// than on a grid at a random phase. Steady beats score 5–10, noise and
// drones about 1.2.
const minBeatContrast = 2

// clearBeat reports whether onsets line up on the bpm grid clearly better
// than on the same grid at an arbitrary phase.
func clearBeat(env []float64, bpm float64) bool {
	period := 60 / bpm * envRate
	best := bestPhaseScore(env, bpm)
	var sum float64
	n := 0
	for p := 0.0; p < period; p += 0.5 {
		sum += phaseScore(env, period, p)
		n++
	}
	return best >= minBeatContrast*sum/float64(n)
}

// bands splits the spectrum for the onset envelope: kick, body, and
// cymbals/noise. Each band's flux is normalised separately, then the low
// band is weighted up, because in most music the kick carries the beat
// while hi-hats and noise would otherwise dominate a broadband sum.
var bands = []struct {
	lo, hi int // FFT bins, inclusive (21.5 Hz each)
	weight float64
}{
	{1, 7, 2},    // ~20–160 Hz
	{8, 93, 1},   // ~160 Hz–2 kHz
	{94, 256, 1}, // ~2–5.5 kHz
}

// onsetEnvelope is the half-wave rectified spectral flux of log magnitudes,
// with its slowly varying level removed so loud sections do not dominate.
// It returns nil when there are no onsets at all.
func onsetEnvelope(samples []float32) []float64 {
	nFrames := (len(samples)-frameSize)/hop + 1
	window := make([]float64, frameSize)
	for i := range window {
		window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/frameSize)
	}

	nBins := frameSize/2 + 1
	prev := make([]float64, nBins)
	cur := make([]float64, nBins)
	buf := make([]complex128, frameSize)
	flux := make([][]float64, len(bands))
	for b := range flux {
		flux[b] = make([]float64, nFrames)
	}
	for f := range nFrames {
		off := f * hop
		for i := range frameSize {
			buf[i] = complex(float64(samples[off+i])*window[i], 0)
		}
		fft(buf)
		for k := range nBins {
			// log(1 + C*|X|) compresses dynamics so quieter attacks count.
			cur[k] = math.Log1p(1000 * cmplx.Abs(buf[k]) / frameSize)
		}
		if f > 0 {
			for b, band := range bands {
				var sum float64
				for k := band.lo; k <= band.hi; k++ {
					sum += max(0, cur[k]-prev[k])
				}
				flux[b][f] = sum
			}
		}
		prev, cur = cur, prev
	}

	env := make([]float64, nFrames)
	used := false
	for b, band := range bands {
		e := detrend(flux[b])
		if e == nil {
			continue
		}
		used = true
		for i, v := range e {
			env[i] += band.weight * v
		}
	}
	if !used {
		return nil
	}
	return env
}

// detrend subtracts a ~0.4 s moving average and rectifies, keeping the
// peaks and dropping sustained energy, then scales to unit mean. It returns
// nil for a flat signal.
func detrend(flux []float64) []float64 {
	n := len(flux)
	half := int(math.Round(0.2 * envRate))
	cum := make([]float64, n+1)
	for i, v := range flux {
		cum[i+1] = cum[i] + v
	}
	env := make([]float64, n)
	var total float64
	for i := range flux {
		lo, hi := max(0, i-half), min(n, i+half+1)
		mean := (cum[hi] - cum[lo]) / float64(hi-lo)
		env[i] = max(0, flux[i]-mean)
		total += env[i]
	}
	if total <= 1e-9 {
		return nil
	}
	scale := float64(n) / total
	for i := range env {
		env[i] *= scale
	}
	return env
}

// coarseTempo picks the best tempo on a 0.5 BPM grid from the envelope's
// autocorrelation.
func coarseTempo(env []float64) float64 {
	maxLag := int(math.Ceil(4*60/MinBPM*envRate)) + 2
	maxLag = min(maxLag, len(env)-1)
	acf := make([]float64, maxLag+1)
	for lag := 1; lag <= maxLag; lag++ {
		var s float64
		for i := 0; i+lag < len(env); i++ {
			s += env[i] * env[i+lag]
		}
		acf[lag] = s / float64(len(env)-lag)
	}
	at := func(lag float64) float64 { // linear interpolation
		i := int(lag)
		if i+1 > maxLag {
			return 0
		}
		f := lag - float64(i)
		return acf[i]*(1-f) + acf[i+1]*f
	}

	best, bestScore := 120.0, 0.0
	for bpm := float64(MinBPM); bpm <= MaxBPM; bpm += 0.5 {
		period := 60 / bpm * envRate
		var s float64
		for k := 1.0; k <= 4; k++ {
			s += at(k * period)
		}
		// Broad log-normal prior around 120 BPM, about an octave wide.
		s *= math.Exp(-0.5 * math.Pow(math.Log2(bpm/120)/1.0, 2))
		if s > bestScore {
			best, bestScore = bpm, s
		}
	}
	return best
}

// fineTempo returns the whole BPM near coarse whose beat grid, at its best
// phase, collects the most onset strength over the whole envelope.
func fineTempo(env []float64, coarse float64) int {
	c := int(math.Round(coarse))
	best, bestScore := c, -1.0
	for bpm := max(MinBPM, c-2); bpm <= min(MaxBPM, c+2); bpm++ {
		if s := bestPhaseScore(env, float64(bpm)); s > bestScore {
			best, bestScore = bpm, s
		}
	}
	return best
}

// fft is an in-place iterative radix-2 FFT; len(x) must be a power of two.
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j |= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		step := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for k := range size / 2 {
				a, b := x[start+k], x[start+k+size/2]*w
				x[start+k], x[start+k+size/2] = a+b, a-b
				w *= step
			}
		}
	}
}

// bestPhaseScore is the most onset strength the beat grid at bpm collects
// over all phases.
func bestPhaseScore(env []float64, bpm float64) float64 {
	period := 60 / bpm * envRate
	var best float64
	for p := 0.0; p < period; p += 0.5 {
		best = max(best, phaseScore(env, period, p))
	}
	return best
}

// phaseScore is the mean of the envelope's local maxima on the grid phase,
// phase+period, ... Taking the maximum within ±1 frame tolerates timing
// jitter and frame quantisation without letting a wrong tempo stay aligned
// for long, since small tempo errors accumulate along the track.
func phaseScore(env []float64, period, phase float64) float64 {
	const tol = 1
	var s float64
	n := 0
	for t := phase; int(t)+tol < len(env); t += period {
		c := int(math.Round(t))
		m := 0.0
		for i := max(0, c-tol); i <= c+tol && i < len(env); i++ {
			m = max(m, env[i])
		}
		s += m
		n++
	}
	if n == 0 {
		return 0
	}
	return s / float64(n)
}
