package motion

import (
	"math"
	"slices"
	"strings"
	"testing"
)

func TestNamesSortedAndComplete(t *testing.T) {
	names := Names()
	if !slices.IsSorted(names) || len(names) != len(BuiltIn) {
		t.Fatalf("Names() = %v", names)
	}
	for _, n := range names {
		m, ok := Get(n)
		if !ok || m.Name != n {
			t.Errorf("Get(%q) = %+v, %v; map key and Name must match", n, m, ok)
		}
		if err := m.Validate(); err != nil {
			t.Errorf("built-in %s invalid: %v", n, err)
		}
	}
	if _, ok := Get(Default); !ok {
		t.Errorf("default %q is not a built-in", Default)
	}
	if _, ok := Get("does-not-exist"); ok {
		t.Error("Get returned ok for unknown motion")
	}
}

func TestStillDoesNotMove(t *testing.T) {
	m, _ := Get("still")
	f := m.MoveFilter(1080, 1080, 120)
	if !strings.Contains(f, "z='1.0000+0.0000*") || !strings.Contains(f, "+0.0*sin") {
		t.Errorf("still should not zoom or shake: %s", f)
	}
	if got := m.PulseExpr(120); !strings.HasPrefix(got, "0.0000*") {
		t.Errorf("still should not pulse: %s", got)
	}
}

func TestMoveFilter(t *testing.T) {
	m, _ := Get("steady") // breath 1% every 8 beats, shake 0.2%

	// 8 beats at 120 BPM = 4 s = 120 frames at 30 fps; shake every 2 beats = 30 frames.
	got := m.MoveFilter(1080, 1080, 120)
	for _, s := range []string{
		"scale=1080:1080:force_original_aspect_ratio=increase",
		"crop=1080:1080",
		"+0.0100*sin(2*PI*on/120.00)",
		"+2.2*sin(2*PI*on/30.00)", // 0.2% of 1080
		"s=1080x1080",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("filter missing %q\n got: %s", s, got)
		}
	}

	// Without a BPM the beats run at AssumedBPM.
	if a, b := m.MoveFilter(1080, 1080, 0), m.MoveFilter(1080, 1080, AssumedBPM); a != b {
		t.Errorf("bpm 0 should match %d BPM:\n%s\n%s", AssumedBPM, a, b)
	}
	// Shake scales with the frame, so a grid tile shakes proportionally less.
	if tile := m.MoveFilter(360, 360, 120); !strings.Contains(tile, "+0.7*sin") {
		t.Errorf("tile shake should be 0.2%% of 360: %s", tile)
	}
}

// The crop-in leaves room for the shake at the breath's smallest zoom, so
// image edges never show.
func TestBaseZoomHidesEdges(t *testing.T) {
	for _, name := range Names() {
		m, _ := Get(name)
		minZoom := m.baseZoom() - m.Breath/100
		margin := (1 - 1/minZoom) / 2 // fraction of the frame on each side
		if margin+1e-9 < m.Shake/100 {
			t.Errorf("%s: margin %.4f smaller than shake %.4f", name, margin, m.Shake/100)
		}
		if minZoom < 1 {
			t.Errorf("%s: zoom dips below 1 (%.4f)", name, minZoom)
		}
	}
}

func TestPulseExpr(t *testing.T) {
	m, _ := Get("steady") // pulse 2% every 4 beats: 2 s at 120 BPM
	if got, want := m.PulseExpr(120), "0.0200*sin(2*PI*t/2.0000)"; got != want {
		t.Errorf("PulseExpr = %q, want %q", got, want)
	}
}

func TestSummary(t *testing.T) {
	m, _ := Get("steady")
	if got, want := m.Summary(), "1% breath every 8 beats, 0.2% shake, 2% pulse every 4 beats"; got != want {
		t.Errorf("Summary = %q, want %q", got, want)
	}
	shifting, _ := Get("shifting")
	if got, want := shifting.Summary(), "1% breath every 8 beats, 0.2% shake, 2% pulse every 4 beats, 10% tint"; got != want {
		t.Errorf("shifting Summary = %q, want %q", got, want)
	}
	still, _ := Get("still")
	if got := still.Summary(); got != "no breath, shake, pulse or tint" {
		t.Errorf("still Summary = %q", got)
	}
}

func TestValidate(t *testing.T) {
	base, _ := Get("steady")
	tests := []struct {
		name    string
		mutate  func(*Motion)
		wantErr []string
	}{
		{"valid", func(*Motion) {}, nil},
		{"missing name", func(m *Motion) { m.Name = "" }, []string{"name is required"}},
		{"zero beats", func(m *Motion) { m.BreathBeats, m.PulseBeats = 0, 0 }, []string{"breath_beats", "pulse_beats"}},
		{"out of range", func(m *Motion) { m.Breath, m.Shake, m.Pulse, m.Tint = 51, 11, 101, 51 }, []string{"breath", "shake", "pulse", "tint"}},
		{"negative", func(m *Motion) { m.Breath, m.Shake, m.Pulse, m.Tint = -1, -1, -1, -1 }, []string{"breath", "shake", "pulse", "tint"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := base
			tt.mutate(&m)
			err := m.Validate()
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error")
			}
			for _, s := range tt.wantErr {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("error missing %q: %v", s, err)
				}
			}
		})
	}
}

// shifting is steady with a tint, so the two only differ in color.
func TestShiftingIsSteadyWithTint(t *testing.T) {
	steady, _ := Get("steady")
	shifting, _ := Get("shifting")
	if shifting.Tint != 10 {
		t.Errorf("shifting tint = %g, want 10", shifting.Tint)
	}
	shifting.Name, shifting.Description, shifting.Tint = steady.Name, steady.Description, 0
	if shifting != steady {
		t.Errorf("shifting should be steady plus a tint:\n%+v\n%+v", shifting, steady)
	}
	for _, name := range Names() {
		if m, _ := Get(name); name != "shifting" && m.Tint != 0 {
			t.Errorf("%s has a tint; only shifting should", name)
		}
	}
}

func TestTinterOff(t *testing.T) {
	still, _ := Get("still")
	tr := still.Tinter(7)
	if tr.On() {
		t.Error("a motion without tint should give a Tinter that is off")
	}
	if got := tr.Cast(3, 0); got != (Cast{1, 1, 1}) {
		t.Errorf("Cast with no tint = %v, want untouched", got)
	}
	if (Tinter{}).On() {
		t.Error("the zero Tinter should be off")
	}
}

// angle is the direction a cast leans in, in radians.
func angle(c Cast) float64 {
	// Inverse of 1 + s*cos(θ - 2πk/3): project onto the cos and sin axes.
	var x, y float64
	for k, g := range c {
		a := 2 * math.Pi * float64(k) / 3
		x += (g - 1) * math.Cos(a)
		y += (g - 1) * math.Sin(a)
	}
	return math.Atan2(y, x)
}

func angleBetween(a, b Cast) float64 {
	d := math.Abs(angle(a) - angle(b))
	return math.Min(d, 2*math.Pi-d)
}

func TestTinterCasts(t *testing.T) {
	m, _ := Get("shifting")
	tr := m.Tinter(42)
	if !tr.On() {
		t.Fatal("shifting should tint")
	}
	for _, period := range []int{0, 1, 2, 3, 7, 24, 100} {
		n := 50
		if period > 0 {
			n = 2 * period
		}
		for i := -n; i < n; i++ {
			c := tr.Cast(i, period)
			// Balanced: brightness, as the mean of the gains, is unchanged.
			if mean := (c[0] + c[1] + c[2]) / 3; math.Abs(mean-1) > 1e-9 {
				t.Fatalf("period %d, cast %d = %v: mean %v, want 1", period, i, c, mean)
			}
			// Full strength, never past it: no channel moves more than 10%,
			// and the cast is always the same size.
			var size float64
			for _, g := range c {
				if math.Abs(g-1) > 0.1+1e-9 {
					t.Fatalf("period %d, cast %d = %v moves a channel more than 10%%", period, i, c)
				}
				size += (g - 1) * (g - 1)
			}
			if want := 0.1 * 0.1 * 1.5; math.Abs(size-want) > 1e-9 {
				t.Fatalf("period %d, cast %d = %v is not at full strength", period, i, c)
			}
			if period > 0 && tr.Cast(i+period, period) != c {
				t.Fatalf("period %d: cast %d does not repeat", period, i)
			}
			// Neighbors, including across the loop point, lean clearly
			// different ways.
			if period != 1 {
				if d := angleBetween(c, tr.Cast(i+1, period)); d < math.Pi/3 {
					t.Errorf("period %d: casts %d and %d only %.0f° apart", period, i, i+1, d*180/math.Pi)
				}
			}
		}
	}
	if tr.Cast(0, 0) != m.Tinter(42).Cast(0, 0) {
		t.Error("the same seed should give the same casts")
	}
	if tr.Cast(0, 0) == m.Tinter(43).Cast(0, 0) {
		t.Error("a different seed should start somewhere else")
	}
}
