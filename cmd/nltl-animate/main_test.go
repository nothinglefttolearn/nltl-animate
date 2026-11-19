package main

import (
	"testing"

	"github.com/nothinglefttolearn/nltl-animate/internal/config"
	"github.com/nothinglefttolearn/nltl-animate/internal/grade"
	"github.com/nothinglefttolearn/nltl-animate/internal/motion"
)

func TestResolveGradeAndMotion(t *testing.T) {
	cfg := &config.Config{
		Grades: []grade.Grade{
			{Name: "mine", Grain: 1},
			{Name: "dark", Grain: 99}, // replaces the built-in
		},
		Motions: []motion.Motion{{Name: "mine", Breath: 7}},
	}

	tests := []struct {
		name      string
		cfg       *config.Config
		grade     string
		wantGrain float64
		wantErr   bool
	}{
		{"custom", cfg, "mine", 1, false},
		{"custom replaces built-in", cfg, "dark", 99, false},
		{"falls back to built-in", cfg, "muted", 25, false},
		{"empty config uses built-in", &config.Config{}, "dark", 40, false},
		{"unknown", cfg, "nope", 0, true},
		{"old preset name", &config.Config{}, "industrial", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g, err := resolveGrade(tt.grade, tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && g.Grain != tt.wantGrain {
				t.Errorf("Grain = %g, want %g", g.Grain, tt.wantGrain)
			}
		})
	}

	if m, err := resolveMotion("mine", cfg); err != nil || m.Breath != 7 {
		t.Errorf("custom motion = %+v, %v", m, err)
	}
	if m, err := resolveMotion("still", cfg); err != nil || m.Breath != 0 {
		t.Errorf("built-in still = %+v, %v", m, err)
	}
	if _, err := resolveMotion("muted", cfg); err == nil {
		t.Error("a grade name is not a motion")
	}
}

func TestLoadConfigEmptyPath(t *testing.T) {
	cfg, err := loadConfig("")
	if err != nil || cfg == nil {
		t.Fatalf("loadConfig(\"\") = %v, %v; want empty config", cfg, err)
	}
}

func TestHeadlessCommand(t *testing.T) {
	base := defaultRenderOptions()
	base.AudioPath, base.ImagePaths = "a.wav", []string{"i.jpg"}
	base.GradeName, base.MotionName = "dark", "gentle"

	full := base
	full.BPM, full.Preview = 138, true
	full.ConfigPath, full.OutputDir = "nltl.yaml", "out"

	several := base
	several.ImagePaths, several.Seed, several.Hold = []string{"a.png", "b.png"}, 42, 3

	gridMixed := base
	gridMixed.Layout, gridMixed.ImagePaths, gridMixed.Seed, gridMixed.Tiles = layoutGrid, []string{"a.png", "b.png"}, 9, tilesMixed

	grid := base
	grid.Layout, grid.GridN = layoutGrid, 4

	train := base
	train.Layout, train.Train.Copies, train.Train.Vanish, train.TrainBG = layoutTrain, 30, 0.7, "#112233"

	tests := []struct {
		name string
		opts renderOptions
		want string
	}{
		{"minimal", base, "nltl-animate --grade dark --motion gentle a.wav i.jpg"},
		{"all flags", full, "nltl-animate --grade dark --motion gentle --bpm 138 --preview --config nltl.yaml --output-dir out a.wav i.jpg"},
		{"several images", several, "nltl-animate --grade dark --motion gentle --hold 3 --seed 42 a.wav a.png b.png"},
		{"grid mixed", gridMixed, "nltl-animate grid --grade dark --motion gentle --seed 9 --n 3 --tiles mixed a.wav a.png b.png"},
		{"grid", grid, "nltl-animate grid --grade dark --motion gentle --n 4 a.wav i.jpg"},
		{"train only changed flags", train, "nltl-animate train --grade dark --motion gentle --copies 30 --vanish 0.7 --bg #112233 a.wav i.jpg"},
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
