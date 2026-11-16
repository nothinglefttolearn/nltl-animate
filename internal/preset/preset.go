package preset

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nothinglefttolearn/nltl-clip/internal/video"
)

type Preset struct {
	Name             string  `yaml:"name"`
	Description      string  `yaml:"description"`
	ZoomBase         float64 `yaml:"zoom_base"`
	ZoomAmp          float64 `yaml:"zoom_amp"`
	ZoomBeats        float64 `yaml:"zoom_beats"`
	JitterAmp        float64 `yaml:"jitter_amp"`
	BlurSigma        float64 `yaml:"blur_sigma"`
	Contrast         float64 `yaml:"contrast"`
	Saturation       float64 `yaml:"saturation"`
	BrightnessAmp    float64 `yaml:"brightness_amp"`
	BrightnessBeats  float64 `yaml:"brightness_beats"`
	VignetteFraction float64 `yaml:"vignette_fraction"` // multiplied by PI
	GrainStrength    int     `yaml:"grain_strength"`
}

var BuiltIn = map[string]Preset{
	"industrial": {
		Name:             "industrial",
		Description:      "Desaturated, grainy, heavy vignette",
		ZoomBase:         1.02,
		ZoomAmp:          0.01,
		ZoomBeats:        8.0,
		JitterAmp:        2.0,
		BlurSigma:        0.6,
		Contrast:         1.15,
		Saturation:       0.7,
		BrightnessAmp:    0.02,
		BrightnessBeats:  4.0,
		VignetteFraction: 0.167,
		GrainStrength:    25,
	},
	"clean": {
		Name:             "clean",
		Description:      "Full colour, minimal grain, subtle movement",
		ZoomBase:         1.01,
		ZoomAmp:          0.005,
		ZoomBeats:        16.0,
		JitterAmp:        1.0,
		BlurSigma:        0.3,
		Contrast:         1.05,
		Saturation:       1.0,
		BrightnessAmp:    0.01,
		BrightnessBeats:  8.0,
		VignetteFraction: 0.10,
		GrainStrength:    8,
	},
	"dark": {
		Name:             "dark",
		Description:      "Deep blacks, extreme grain, maximum vignette",
		ZoomBase:         1.03,
		ZoomAmp:          0.015,
		ZoomBeats:        4.0,
		JitterAmp:        3.0,
		BlurSigma:        0.8,
		Contrast:         1.30,
		Saturation:       0.4,
		BrightnessAmp:    0.04,
		BrightnessBeats:  2.0,
		VignetteFraction: 0.25,
		GrainStrength:    40,
	},
}

func Get(name string) (Preset, bool) {
	p, ok := BuiltIn[name]
	return p, ok
}

func Names() []string {
	names := make([]string, 0, len(BuiltIn))
	for k := range BuiltIn {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// Validate reports every field outside the range the generated ffmpeg filter
// graph accepts. Ranges follow the ffmpeg filters each field feeds into.
func (p Preset) Validate() error {
	var problems []string
	check := func(ok bool, format string, args ...any) {
		if !ok {
			problems = append(problems, fmt.Sprintf(format, args...))
		}
	}

	check(p.Name != "", "name is required")
	// zoompan clamps z to [1, 10]; below 1 would silently do nothing.
	check(p.ZoomBase >= 1 && p.ZoomBase <= 10, "zoom_base must be between 1 and 10, got %g", p.ZoomBase)
	check(p.ZoomAmp >= 0, "zoom_amp must be >= 0, got %g", p.ZoomAmp)
	// Beat counts are divisors in BPM mode; 0 would put Inf into the filter.
	check(p.ZoomBeats > 0, "zoom_beats must be > 0, got %g", p.ZoomBeats)
	check(p.BrightnessBeats > 0, "brightness_beats must be > 0, got %g", p.BrightnessBeats)
	check(p.JitterAmp >= 0, "jitter_amp must be >= 0, got %g", p.JitterAmp)
	check(p.BlurSigma >= 0 && p.BlurSigma <= 1024, "blur_sigma must be between 0 and 1024, got %g", p.BlurSigma)
	check(p.Contrast >= -1000 && p.Contrast <= 1000, "contrast must be between -1000 and 1000, got %g", p.Contrast)
	check(p.Saturation >= 0 && p.Saturation <= 3, "saturation must be between 0 and 3, got %g", p.Saturation)
	check(p.BrightnessAmp >= 0 && p.BrightnessAmp <= 1, "brightness_amp must be between 0 and 1, got %g", p.BrightnessAmp)
	// vignette angle is limited to [0, PI/2].
	check(p.VignetteFraction >= 0 && p.VignetteFraction <= 0.5,
		"vignette_fraction must be between 0 and 0.5, got %g", p.VignetteFraction)
	check(p.GrainStrength >= 0 && p.GrainStrength <= 100, "grain_strength must be between 0 and 100, got %d", p.GrainStrength)

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("preset %q:\n  - %s", p.Name, strings.Join(problems, "\n  - "))
}

// BuildFilter returns the full single-image filter chain: motion then grade.
func (p Preset) BuildFilter(width, height, bpm int) string {
	return p.MotionFilter(width, height, bpm) + "," + p.GradeFilter(bpm)
}

// MotionFilter scales and crops the input to width x height and applies the
// zoom "breathing" and jitter. It expects one input frame per output frame.
func (p Preset) MotionFilter(width, height, bpm int) string {
	var zoomExpr, jitterXExpr, jitterYExpr string

	if bpm > 0 {
		// BPM-synced: periods in output frames.
		zoomFramePeriod := p.ZoomBeats * 60.0 / float64(bpm) * video.FPS
		zoomExpr = fmt.Sprintf("%.4f+%.4f*sin(2*PI*on/%.2f)", p.ZoomBase, p.ZoomAmp, zoomFramePeriod)

		jitterFramePeriod := 2.0 * 60.0 / float64(bpm) * video.FPS
		jitterXExpr = fmt.Sprintf("iw/2-(iw/zoom/2)+%.1f*sin(2*PI*on/%.2f)", p.JitterAmp, jitterFramePeriod)
		jitterYExpr = fmt.Sprintf("ih/2-(ih/zoom/2)+%.1f*cos(2*PI*on/%.2f)", p.JitterAmp, jitterFramePeriod*0.9)
	} else {
		// Time-based defaults matching the original script feel
		zoomExpr = fmt.Sprintf("%.4f+%.4f*sin(2*PI*on/200)", p.ZoomBase, p.ZoomAmp)
		jitterXExpr = fmt.Sprintf("iw/2-(iw/zoom/2)+%.1f*sin(on/7)", p.JitterAmp)
		jitterYExpr = fmt.Sprintf("ih/2-(ih/zoom/2)+%.1f*cos(on/9)", p.JitterAmp)
	}

	return fmt.Sprintf(
		"scale=%d:%d:force_original_aspect_ratio=increase,"+
			"crop=%d:%d,"+
			"zoompan=z='%s':x='%s':y='%s':d=1:fps=%d:s=%dx%d",
		width, height,
		width, height,
		zoomExpr, jitterXExpr, jitterYExpr, video.FPS, width, height,
	)
}

// GradeFilter applies blur, the brightness pulse, contrast/saturation,
// vignette and grain. It works at whatever size its input is.
func (p Preset) GradeFilter(bpm int) string {
	brightnessExpr := fmt.Sprintf("%.4f*sin(2*PI*t*0.6)", p.BrightnessAmp)
	if bpm > 0 {
		brightnessFreq := float64(bpm) / (p.BrightnessBeats * 60.0)
		brightnessExpr = fmt.Sprintf("%.4f*sin(2*PI*t*%.4f)", p.BrightnessAmp, brightnessFreq)
	}

	return fmt.Sprintf(
		"gblur=sigma=%.1f:steps=1,"+
			"eq=brightness='%s':contrast=%.2f:saturation=%.2f,"+
			"vignette=PI*%.3f,"+
			"noise=alls=%d:allf=t+u",
		p.BlurSigma,
		brightnessExpr, p.Contrast, p.Saturation,
		p.VignetteFraction,
		p.GrainStrength,
	)
}
