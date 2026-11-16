package preset

import (
	"slices"
	"strings"
	"testing"
)

func TestNamesSortedAndComplete(t *testing.T) {
	names := Names()
	if !slices.IsSorted(names) {
		t.Errorf("Names() not sorted: %v", names)
	}
	if len(names) != len(BuiltIn) {
		t.Fatalf("Names() returned %d names, want %d", len(names), len(BuiltIn))
	}
	for _, n := range names {
		p, ok := Get(n)
		if !ok {
			t.Errorf("Get(%q) not found", n)
		}
		if p.Name != n {
			t.Errorf("BuiltIn[%q].Name = %q; map key and Name must match", n, p.Name)
		}
	}
}

func TestGetUnknown(t *testing.T) {
	if _, ok := Get("does-not-exist"); ok {
		t.Error("Get returned ok for unknown preset")
	}
}

func TestBuildFilter(t *testing.T) {
	p, _ := Get("industrial")

	tests := []struct {
		name           string
		bpm            int
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "time-based",
			bpm:  0,
			wantContains: []string{
				"scale=1080:1920:force_original_aspect_ratio=increase",
				"crop=1080:1920",
				"zoompan=z='1.0200+0.0100*sin(2*PI*on/200)'",
				"s=1080x1920",
				"sin(2*PI*t*0.6)",
				"noise=alls=25:allf=t+u",
			},
		},
		{
			// 8 beats at 120 BPM = 4s = 120 frames at 30fps.
			name: "bpm-synced",
			bpm:  120,
			wantContains: []string{
				"sin(2*PI*on/120.00)",
				// brightness: 120 / (4 * 60) = 0.5 Hz
				"sin(2*PI*t*0.5000)",
			},
			wantNotContain: []string{"on/200"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.BuildFilter(1080, 1920, tt.bpm)
			for _, s := range tt.wantContains {
				if !strings.Contains(got, s) {
					t.Errorf("filter missing %q\n got: %s", s, got)
				}
			}
			for _, s := range tt.wantNotContain {
				if strings.Contains(got, s) {
					t.Errorf("filter unexpectedly contains %q\n got: %s", s, got)
				}
			}
		})
	}
}

func TestBuiltInsValid(t *testing.T) {
	for _, name := range Names() {
		p, _ := Get(name)
		if err := p.Validate(); err != nil {
			t.Errorf("built-in %s invalid: %v", name, err)
		}
	}
}

func TestValidate(t *testing.T) {
	base, _ := Get("industrial")

	tests := []struct {
		name    string
		mutate  func(*Preset)
		wantErr []string
	}{
		{"valid", func(*Preset) {}, nil},
		{"missing name", func(p *Preset) { p.Name = "" }, []string{"name is required"}},
		{"zero beats", func(p *Preset) { p.ZoomBeats, p.BrightnessBeats = 0, 0 },
			[]string{"zoom_beats", "brightness_beats"}},
		{"zoom below 1", func(p *Preset) { p.ZoomBase = 0.5 }, []string{"zoom_base"}},
		{"out of range", func(p *Preset) {
			p.Saturation, p.VignetteFraction, p.GrainStrength, p.BrightnessAmp = 4, 0.6, 101, -0.1
		}, []string{"saturation", "vignette_fraction", "grain_strength", "brightness_amp"}},
		{"negative amplitudes", func(p *Preset) { p.ZoomAmp, p.JitterAmp, p.BlurSigma = -1, -1, -1 },
			[]string{"zoom_amp", "jitter_amp", "blur_sigma"}},
		{"contrast", func(p *Preset) { p.Contrast = 2000 }, []string{"contrast"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := base
			tt.mutate(&p)
			err := p.Validate()
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
