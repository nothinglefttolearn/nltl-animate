// Package motion is the part of a clip's treatment that follows the beat:
// breath (zoom in and out), shake (drift and jitter) and pulse (brightness
// swell). See CONTEXT.md.
package motion

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nothinglefttolearn/nltl-animate/internal/video"
)

// AssumedBPM is the tempo the beat dials run at when the BPM is unknown.
const AssumedBPM = 120

// shakeBeats is how many beats one shake cycle takes. It is fixed rather than
// a dial: shake reads as texture, not rhythm.
const shakeBeats = 2.0

// Motion is a named set of the five motion dials. Strengths are percentages
// of what you see; rates are in beats.
type Motion struct {
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Breath      float64 `yaml:"breath"`       // zoom in and out, % of the frame
	BreathBeats float64 `yaml:"breath_beats"` // beats per breath
	Shake       float64 `yaml:"shake"`        // drift and jitter, % of the frame
	Pulse       float64 `yaml:"pulse"`        // brightness swell, %
	PulseBeats  float64 `yaml:"pulse_beats"`  // beats per pulse
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
	if m.Breath == 0 && m.Shake == 0 && m.Pulse == 0 {
		return "no breath, shake or pulse"
	}
	parts := []string{
		fmt.Sprintf("%g%% breath every %g beats", m.Breath, m.BreathBeats),
		fmt.Sprintf("%g%% shake", m.Shake),
		fmt.Sprintf("%g%% pulse every %g beats", m.Pulse, m.PulseBeats),
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
