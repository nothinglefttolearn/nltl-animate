// Package motion is the part of a clip's treatment that follows the beat:
// breath (zoom in and out), shake (drift and jitter), pulse (brightness
// swell) and tint (a color cast per appearance of an image). See CONTEXT.md
// and docs/adr/0002-tint-lives-in-motion.md.
package motion

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"

	"github.com/nothinglefttolearn/nltl-animate/internal/video"
)

// AssumedBPM is the tempo the beat dials run at when the BPM is unknown.
const AssumedBPM = 120

// shakeBeats is how many beats one shake cycle takes. It is fixed rather than
// a dial: shake reads as texture, not rhythm.
const shakeBeats = 2.0

// Motion is a named set of the six motion dials. Strengths are percentages
// of what you see; rates are in beats.
type Motion struct {
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Breath      float64 `yaml:"breath"`       // zoom in and out, % of the frame
	BreathBeats float64 `yaml:"breath_beats"` // beats per breath
	Shake       float64 `yaml:"shake"`        // drift and jitter, % of the frame
	Pulse       float64 `yaml:"pulse"`        // brightness swell, %
	PulseBeats  float64 `yaml:"pulse_beats"`  // beats per pulse
	Tint        float64 `yaml:"tint"`         // color cast per appearance of an image, %
}

// Default is the motion used when none is chosen.
const Default = "steady"

var BuiltIn = map[string]Motion{
	"still": {
		Name: "still", Description: "No movement at all",
		BreathBeats: 8, PulseBeats: 4,
	},
	"gentle": {
		Name: "gentle", Description: "Slow, barely-there drift",
		Breath: 0.5, BreathBeats: 16, Shake: 0.1, Pulse: 1, PulseBeats: 8,
	},
	"steady": {
		Name: "steady", Description: "Noticeable breathing that keeps time",
		Breath: 1, BreathBeats: 8, Shake: 0.2, Pulse: 2, PulseBeats: 4,
	},
	"restless": {
		Name: "restless", Description: "Fast, deep breathing and a strong pulse",
		Breath: 1.5, BreathBeats: 4, Shake: 0.3, Pulse: 4, PulseBeats: 2,
	},
	"shifting": {
		Name: "shifting", Description: "Steady, with every image in a different color",
		Breath: 1, BreathBeats: 8, Shake: 0.2, Pulse: 2, PulseBeats: 4, Tint: 10,
	},
}

func Get(name string) (Motion, bool) {
	m, ok := BuiltIn[name]
	return m, ok
}

func Names() []string {
	names := make([]string, 0, len(BuiltIn))
	for k := range BuiltIn {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// Summary states the dial values in plain words.
func (m Motion) Summary() string {
	if m.Breath == 0 && m.Shake == 0 && m.Pulse == 0 && m.Tint == 0 {
		return "no breath, shake, pulse or tint"
	}
	parts := []string{
		fmt.Sprintf("%g%% breath every %g beats", m.Breath, m.BreathBeats),
		fmt.Sprintf("%g%% shake", m.Shake),
		fmt.Sprintf("%g%% pulse every %g beats", m.Pulse, m.PulseBeats),
	}
	if m.Tint > 0 {
		parts = append(parts, fmt.Sprintf("%g%% tint", m.Tint))
	}
	return strings.Join(parts, ", ")
}

// Validate reports every dial outside its range.
func (m Motion) Validate() error {
	var problems []string
	check := func(ok bool, format string, args ...any) {
		if !ok {
			problems = append(problems, fmt.Sprintf(format, args...))
		}
	}
	check(m.Name != "", "name is required")
	check(m.Breath >= 0 && m.Breath <= 50, "breath must be between 0 and 50, got %g", m.Breath)
	check(m.BreathBeats > 0, "breath_beats must be > 0, got %g", m.BreathBeats)
	check(m.Shake >= 0 && m.Shake <= 10, "shake must be between 0 and 10, got %g", m.Shake)
	check(m.Pulse >= 0 && m.Pulse <= 100, "pulse must be between 0 and 100, got %g", m.Pulse)
	check(m.PulseBeats > 0, "pulse_beats must be > 0, got %g", m.PulseBeats)
	check(m.Tint >= 0 && m.Tint <= 50, "tint must be between 0 and 50, got %g", m.Tint)
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("motion %q:\n  - %s", m.Name, strings.Join(problems, "\n  - "))
}

// framesPerBeat at bpm, or at AssumedBPM when bpm is 0.
func framesPerBeat(bpm int) float64 {
	if bpm <= 0 {
		bpm = AssumedBPM
	}
	return 60.0 / float64(bpm) * video.FPS
}

// baseZoom is the constant crop-in that keeps the image edges out of view:
// enough margin for the shake at the breath's smallest zoom.
func (m Motion) baseZoom() float64 {
	return 1/(1-2*m.Shake/100) + m.Breath/100
}

// MoveFilter scales and crops the input to width x height and applies the
// breath and shake. It expects one input frame per output frame.
func (m Motion) MoveFilter(width, height, bpm int) string {
	fpb := framesPerBeat(bpm)
	breathPeriod := m.BreathBeats * fpb
	shakePeriod := shakeBeats * fpb
	shakeX := m.Shake / 100 * float64(width)
	shakeY := m.Shake / 100 * float64(height)

	zoom := fmt.Sprintf("%.4f+%.4f*sin(2*PI*on/%.2f)", m.baseZoom(), m.Breath/100, breathPeriod)
	x := fmt.Sprintf("iw/2-(iw/zoom/2)+%.1f*sin(2*PI*on/%.2f)", shakeX, shakePeriod)
	y := fmt.Sprintf("ih/2-(ih/zoom/2)+%.1f*cos(2*PI*on/%.2f)", shakeY, shakePeriod*0.9)

	return fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=increase,"+
			"crop=%d:%d,"+
			"zoompan=z='%s':x='%s':y='%s':d=1:fps=%d:s=%dx%d",
		width, height,
		width, height,
		zoom, x, y, video.FPS, width, height,
	)
}

// PulseExpr is the ffmpeg brightness expression for the pulse, in time t
// (seconds), for the eq filter's brightness option.
func (m Motion) PulseExpr(bpm int) string {
	periodSeconds := m.PulseBeats * framesPerBeat(bpm) / video.FPS
	return fmt.Sprintf("%.4f*sin(2*PI*t/%.4f)", m.Pulse/100, periodSeconds)
}

// Cast is the gain on red, green and blue for one appearance of an image;
// {1, 1, 1} leaves it untouched.
type Cast [3]float64

// Tinter gives each appearance of an image its cast (see Motion.Tinter). The
// zero Tinter is off.
type Tinter struct {
	strength float64 // largest change of one channel, as a fraction
	start    float64 // direction of appearance 0, radians
}

// Tinter returns the casts for this motion's tint. The seed picks where on
// the color wheel they start.
func (m Motion) Tinter(seed uint64) Tinter {
	r := rand.New(rand.NewPCG(seed, 0x74696e74)) //nolint:gosec // visual randomness, not security
	return Tinter{strength: m.Tint / 100, start: r.Float64() * 2 * math.Pi}
}

// On reports whether the casts change anything.
func (t Tinter) On() bool { return t.strength > 0 }

// goldenAngle is the turn between consecutive casts: far from the last one,
// and never landing near an earlier one, so the casts spread evenly.
var goldenAngle = math.Pi * (3 - math.Sqrt(5)) // about 137.5°

// Cast is the cast for appearance i (any integer). Every cast is at full
// strength and balanced, so it changes the color but not the brightness; it
// leans a big step around the color wheel from cast i-1.
//
// For a clip that loops after period appearances, pass period so cast
// i+period equals cast i, and the step from the last cast back to the first
// is as big as the others. With period 0 the casts never repeat.
func (t Tinter) Cast(i, period int) Cast {
	if !t.On() {
		return Cast{1, 1, 1}
	}
	var turn float64
	if period > 0 {
		turn = 2 * math.Pi * float64(posMod(i*loopStep(period), period)) / float64(period)
	} else {
		turn = float64(i) * goldenAngle
	}
	theta := t.start + turn
	// Three gains 120° apart around the wheel always add up to 3, so the
	// average stays 1.
	var c Cast
	for k := range c {
		c[k] = 1 + t.strength*math.Cos(theta-2*math.Pi*float64(k)/3)
	}
	return c
}

// loopStep is the golden-angle step for a loop of period casts: a whole
// number of period-ths of a turn, near the golden fraction, and coprime with
// period so every cast in the loop is different.
func loopStep(period int) int {
	golden := goldenAngle / (2 * math.Pi) // about 0.382
	k := max(1, int(math.Round(float64(period)*golden)))
	for d := 0; ; d++ {
		for _, c := range []int{k + d, k - d} {
			if c >= 1 && gcd(c, period) == 1 {
				return c
			}
		}
	}
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// posMod is a mod b in [0, b).
func posMod(a, b int) int { return ((a % b) + b) % b }
