package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nltl.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadAndGetPreset(t *testing.T) {
	cfg, err := Load(writeFile(t, `
presets:
  - name: mine
    zoom_base: 1.05
    grain_strength: 12
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, ok := cfg.GetPreset("mine")
	if !ok {
		t.Fatal("GetPreset(mine) not found")
	}
	if p.ZoomBase != 1.05 || p.GrainStrength != 12 {
		t.Errorf("got %+v", p)
	}
	if _, ok := cfg.GetPreset("missing"); ok {
		t.Error("GetPreset(missing) returned ok")
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Error("expected error for missing file")
	}
	if _, err := Load(writeFile(t, "presets: [this is: not valid")); err == nil {
		t.Error("expected error for invalid YAML")
	}
}
