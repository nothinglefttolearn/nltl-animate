package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nothinglefttolearn/nltl-animate/internal/grade"
	"github.com/nothinglefttolearn/nltl-animate/internal/motion"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nltl.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadInheritsFromDefaults(t *testing.T) {
	cfg, err := Load(writeFile(t, `
grades:
  - name: mine
    grain: 12
motions:
  - name: mine
    breath: 3
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	g, ok := cfg.GetGrade("mine")
	if !ok {
		t.Fatal("GetGrade(mine) not found")
	}
	wantG, _ := grade.Get(grade.Default)
	wantG.Name, wantG.Description, wantG.Grain = "mine", "", 12
	if g != wantG {
		t.Errorf("grade got  %+v\nwant %+v", g, wantG)
	}

	m, ok := cfg.GetMotion("mine")
	if !ok {
		t.Fatal("GetMotion(mine) not found")
	}
	wantM, _ := motion.Get(motion.Default)
	wantM.Name, wantM.Description, wantM.Breath = "mine", "", 3
	if m != wantM {
		t.Errorf("motion got  %+v\nwant %+v", m, wantM)
	}

	if _, ok := cfg.GetGrade("missing"); ok {
		t.Error("GetGrade(missing) returned ok")
	}
	if _, ok := cfg.GetMotion("missing"); ok {
		t.Error("GetMotion(missing) returned ok")
	}
}

func TestLoadExtends(t *testing.T) {
	cfg, err := Load(writeFile(t, `
grades:
  - name: darker
    extends: dark
    contrast: 50
  - name: darkest
    extends: darker
    saturation: -100
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	dark, _ := grade.Get("dark")

	darker, _ := cfg.GetGrade("darker")
	if darker.Contrast != 50 || darker.Grain != dark.Grain {
		t.Errorf("darker = %+v; want dark with contrast 50", darker)
	}
	darkest, _ := cfg.GetGrade("darkest")
	if darkest.Contrast != 50 || darkest.Saturation != -100 || darkest.Grain != dark.Grain {
		t.Errorf("darkest = %+v; want darker with saturation -100", darkest)
	}
}

func TestLoadEmpty(t *testing.T) {
	cfg, err := Load(writeFile(t, ""))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Grades) != 0 || len(cfg.Motions) != 0 {
		t.Errorf("got %+v", cfg)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr []string
	}{
		{"invalid yaml", "grades: [this is: not valid", []string{"parsing"}},
		{"old presets", "presets:\n  - name: a\n    zoom_amp: 0.1\n", []string{`"presets" is no longer supported`, "grades:", "motions:"}},
		{"unknown field", "grades:\n  - name: a\n    grian: 10\n", []string{"grian"}},
		{"motion dial in a grade", "grades:\n  - name: a\n    breath: 1\n", []string{"breath"}},
		{"unknown top-level key", "grade:\n  - name: a\n", []string{"grade"}},
		{"wrong type", "grades:\n  - name: a\n    grain: lots\n", []string{"line 3", "lots"}},
		{"missing name", "motions:\n  - breath: 1\n", []string{"motion 1", "name is required"}},
		{"duplicate", "grades:\n  - name: a\n  - name: a\n", []string{"duplicate", `"a"`}},
		{"unknown base", "grades:\n  - name: a\n    extends: nope\n", []string{`extends unknown grade "nope"`}},
		{"extends other kind", "motions:\n  - name: a\n    extends: muted\n", []string{`extends unknown motion "muted"`}},
		{"forward reference", "grades:\n  - name: a\n    extends: b\n  - name: b\n", []string{`"b"`}},
		{"invalid values", "motions:\n  - name: a\n    pulse_beats: 0\n    shake: 90\n",
			[]string{"pulse_beats", "shake"}},
		{"reports every one", "grades:\n  - name: a\n    saturation: 900\nmotions:\n  - name: b\n    breath: 500\n",
			[]string{`grade "a"`, `motion "b"`}},
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
	for _, name := range []string{"my-grade", "my-darker"} {
		if _, ok := cfg.GetGrade(name); !ok {
			t.Errorf("example missing grade %q", name)
		}
	}
	if _, ok := cfg.GetMotion("slow-breath"); !ok {
		t.Error("example missing motion slow-breath")
	}
}
