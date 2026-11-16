package main

import (
	"testing"

	"github.com/nothinglefttolearn/nltl-clip/internal/config"
	"github.com/nothinglefttolearn/nltl-clip/internal/preset"
)

func TestResolvePreset(t *testing.T) {
	cfg := &config.Config{Presets: []preset.Preset{
		{Name: "mine", GrainStrength: 1},
		{Name: "dark", GrainStrength: 99}, // shadows the built-in
	}}

	tests := []struct {
		name      string
		cfg       *config.Config
		preset    string
		wantGrain int
		wantErr   bool
	}{
		{"custom", cfg, "mine", 1, false},
		{"custom shadows built-in", cfg, "dark", 99, false},
		{"falls back to built-in", cfg, "industrial", 25, false},
		{"empty config uses built-in", &config.Config{}, "dark", 40, false},
		{"unknown", cfg, "nope", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := resolvePreset(tt.preset, tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && p.GrainStrength != tt.wantGrain {
				t.Errorf("GrainStrength = %d, want %d", p.GrainStrength, tt.wantGrain)
			}
		})
	}
}

func TestLoadConfigEmptyPath(t *testing.T) {
	cfg, err := loadConfig("")
	if err != nil || cfg == nil {
		t.Fatalf("loadConfig(\"\") = %v, %v; want empty config", cfg, err)
	}
}

func TestHeadlessCommand(t *testing.T) {
	base := renderOptions{
		AudioPath: "a.wav", ImagePath: "i.jpg",
		ProfileName: "youtube", PresetName: "dark",
	}
	full := base
	full.BPM, full.Still, full.Preview = 138, true, true
	full.ConfigPath, full.OutputDir = "nltl.yaml", "out"

	tests := []struct {
		name string
		opts renderOptions
		want string
	}{
		{"minimal", base, "nltl-clip --profile youtube --preset dark a.wav i.jpg"},
		{"all flags", full, "nltl-clip --profile youtube --preset dark --bpm 138 --still --preview --config nltl.yaml --output-dir out a.wav i.jpg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opts.headlessCommand(); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestBuildVersionInjected(t *testing.T) {
	orig := version
	t.Cleanup(func() { version = orig })

	version = "v9.9.9"
	if got := buildVersion(); got != "v9.9.9" {
		t.Errorf("buildVersion() = %q, want injected v9.9.9", got)
	}
	version = ""
	if got := buildVersion(); got == "" {
		t.Error("buildVersion() returned empty string")
	}
}
