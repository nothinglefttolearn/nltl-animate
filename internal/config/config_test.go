package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nothinglefttolearn/nltl-animate/internal/preset"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nltl.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadInheritsFromDefaultBase(t *testing.T) {
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

	want, _ := preset.Get(DefaultBase)
	want.Name, want.Description = "mine", ""
	want.ZoomBase, want.GrainStrength = 1.05, 12
	if p != want {
		t.Errorf("got  %+v\nwant %+v", p, want)
	}
	if _, ok := cfg.GetPreset("missing"); ok {
		t.Error("GetPreset(missing) returned ok")
	}
}

func TestLoadExtends(t *testing.T) {
	cfg, err := Load(writeFile(t, `
presets:
  - name: darker
    extends: dark
    contrast: 1.5
  - name: darkest
    extends: darker
    saturation: 0
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	dark, _ := preset.Get("dark")

	darker, _ := cfg.GetPreset("darker")
	if darker.Contrast != 1.5 || darker.GrainStrength != dark.GrainStrength {
		t.Errorf("darker = %+v", darker)
	}
	darkest, _ := cfg.GetPreset("darkest")
	if darkest.Saturation != 0 || darkest.Contrast != 1.5 || darkest.GrainStrength != dark.GrainStrength {
		t.Errorf("darkest = %+v", darkest)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	cfg, err := Load(writeFile(t, ""))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Presets) != 0 {
		t.Errorf("got %d presets", len(cfg.Presets))
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Error("expected error for missing file")
	}

	tests := []struct {
		name    string
		yaml    string
		wantErr []string
	}{
		{"invalid yaml", "presets: [this is: not valid", []string{"parsing"}},
		{"unknown field", "presets:\n  - name: a\n    zoom_amplitude: 0.1\n", []string{"zoom_amplitude"}},
		{"unknown top-level key", "preset:\n  - name: a\n", []string{"preset"}},
		{"wrong type", "presets:\n  - name: a\n    grain_strength: lots\n", []string{"line 3", "lots"}},
		{"missing name", "presets:\n  - contrast: 1.1\n", []string{"preset 1", "name is required"}},
		{"duplicate", "presets:\n  - name: a\n  - name: a\n", []string{"duplicate", `"a"`}},
		{"unknown base", "presets:\n  - name: a\n    extends: nope\n", []string{`extends unknown preset "nope"`}},
		{"forward reference", "presets:\n  - name: a\n    extends: b\n  - name: b\n", []string{`"b"`}},
		{"invalid values", "presets:\n  - name: a\n    brightness_beats: 0\n    saturation: 9\n",
			[]string{"brightness_beats", "saturation"}},
		{"reports every preset", "presets:\n  - name: a\n    saturation: 9\n  - name: b\n    grain_strength: 500\n",
			[]string{`preset "a"`, `preset "b"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(writeFile(t, tt.yaml))
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

func TestExampleConfigLoads(t *testing.T) {
	cfg, err := Load(filepath.Join("..", "..", "nltl.yaml.example"))
	if err != nil {
		t.Fatalf("nltl.yaml.example must stay valid: %v", err)
	}
	for _, name := range []string{"my-preset", "my-darker"} {
		if _, ok := cfg.GetPreset(name); !ok {
			t.Errorf("example missing preset %q", name)
		}
	}
}
