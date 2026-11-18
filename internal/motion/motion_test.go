package motion

import (
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
	still, _ := Get("still")
	if got := still.Summary(); got != "no breath, shake or pulse" {
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
		{"out of range", func(m *Motion) { m.Breath, m.Shake, m.Pulse = 51, 11, 101 }, []string{"breath", "shake", "pulse"}},
		{"negative", func(m *Motion) { m.Breath, m.Shake, m.Pulse = -1, -1, -1 }, []string{"breath", "shake", "pulse"}},
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
