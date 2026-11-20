// Package grade is the fixed color and texture part of a clip's treatment:
// contrast, saturation, softness, vignette and grain. Nothing in a grade
// follows the beat. See CONTEXT.md.
package grade

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Grade is a named set of the five grade dials. Every dial is a percentage
// and 0 leaves the image untouched.
type Grade struct {
	Name        string  `yaml:"name"`
	Description string  `yaml:"description"`
	Contrast    float64 `yaml:"contrast"`   // % change, -100 (flat gray) to 100
	Saturation  float64 `yaml:"saturation"` // % change, -100 (black and white) to 100
	Softness    float64 `yaml:"softness"`   // blur, 0 (sharp) to 100 (heavily smeared)
	Vignette    float64 `yaml:"vignette"`   // corner darkening, 0 (none) to 100
	Grain       float64 `yaml:"grain"`      // film grain, 0 (clean) to 100
}

// Default is the grade used when none is chosen.
const Default = "muted"

var BuiltIn = map[string]Grade{
	"clean": {
		Name: "clean", Description: "Full color, light vignette, a trace of grain",
		Contrast: 5, Saturation: 0, Softness: 3, Vignette: 20, Grain: 8,
	},
	"muted": {
		Name: "muted", Description: "Pulled-back color, visible grain, medium vignette",
		Contrast: 15, Saturation: -30, Softness: 6, Vignette: 33, Grain: 25,
	},
	"dark": {
		Name: "dark", Description: "Little color, hard contrast, heavy grain and vignette",
		Contrast: 30, Saturation: -60, Softness: 8, Vignette: 50, Grain: 40,
	},
	"vivid": {
		Name: "vivid", Description: "Boosted color, crisp and clean",
		Contrast: 20, Saturation: 45, Softness: 0, Vignette: 15, Grain: 5,
	},
	"faded": {
		Name: "faded", Description: "Flat, washed-out color, like an old print",
		Contrast: -30, Saturation: -20, Softness: 5, Vignette: 10, Grain: 15,
	},
	"dream": {
		Name: "dream", Description: "Hazy and smeared, soft dark corners",
		Contrast: -10, Saturation: 10, Softness: 40, Vignette: 40, Grain: 10,
	},
	"mono": {
		Name: "mono", Description: "Black and white, punchy, with grain",
		Contrast: 25, Saturation: -100, Softness: 4, Vignette: 45, Grain: 30,
	},
	"grit": {
		Name: "grit", Description: "Sharp, with very heavy grain",
		Contrast: 20, Saturation: -20, Softness: 0, Vignette: 30, Grain: 80,
	},
	"tunnel": {
		Name: "tunnel", Description: "A spotlight: everything near the edges falls to black",
		Contrast: 10, Saturation: -10, Softness: 5, Vignette: 90, Grain: 20,
	},
}

func Get(name string) (Grade, bool) {
	g, ok := BuiltIn[name]
	return g, ok
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
func (g Grade) Summary() string {
	return fmt.Sprintf("contrast %s, saturation %s, softness %g, vignette %g, grain %g",
		signed(g.Contrast), signed(g.Saturation), g.Softness, g.Vignette, g.Grain)
}

func signed(v float64) string {
	if v > 0 {
		return fmt.Sprintf("+%g", v)
	}
	return fmt.Sprintf("%g", v)
}

// Validate reports every dial outside its range.
func (g Grade) Validate() error {
	var problems []string
	check := func(v, lo, hi float64, name string) {
		if v < lo || v > hi {
			problems = append(problems, fmt.Sprintf("%s must be between %g and %g, got %g", name, lo, hi, v))
		}
	}
	if g.Name == "" {
		problems = append(problems, "name is required")
	}
	check(g.Contrast, -100, 100, "contrast")
	check(g.Saturation, -100, 100, "saturation")
	check(g.Softness, 0, 100, "softness")
	check(g.Vignette, 0, 100, "vignette")
	check(g.Grain, 0, 100, "grain")
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("grade %q:\n  - %s", g.Name, strings.Join(problems, "\n  - "))
}

// Filter applies the grade at whatever size its input is. brightness is an
// ffmpeg expression for the eq filter's brightness (the motion's pulse, or
// "0"); it shares the eq filter so the pulse costs no extra pass. eq
// evaluates its expressions only once unless told to per frame.
func (g Grade) Filter(brightness string) string {
	return fmt.Sprintf(
		"gblur=sigma=%.1f:steps=1,"+
			"eq=brightness='%s':contrast=%.2f:saturation=%.2f:eval=frame,"+
			"vignette=angle=%.6f,"+
			"noise=alls=%d:allf=t+u",
		g.Softness/10, // 100 → sigma 10, heavily smeared at 1080
		brightness, 1+g.Contrast/100, 1+g.Saturation/100,
		g.Vignette/100*math.Pi/2, // ffmpeg's largest vignette angle
		int(math.Round(g.Grain)),
	)
}
