package interactive

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nothinglefttolearn/nltl-animate/internal/grade"
	"github.com/nothinglefttolearn/nltl-animate/internal/motion"
)

func TestNamedOptions(t *testing.T) {
	custom := []grade.Grade{{Name: "mine"}, {Name: "dark"}, {Name: "mine"}}
	opts, names, sums := namedOptions(custom, grade.Names(), grade.Get,
		func(g grade.Grade) (string, string, string) { return g.Name, g.Description, g.Summary() })

	// Custom first, then the built-ins not shadowed by a custom one.
	want := append([]string{"mine", "dark"}, slices.DeleteFunc(grade.Names(), func(n string) bool { return n == "dark" })...)
	if !slices.Equal(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
	if len(opts) != len(names) {
		t.Errorf("got %d options for %d names", len(opts), len(names))
	}

	// The dial values follow the name; a custom grade shadows the built-in.
	if muted, _ := grade.Get("muted"); sums["muted"] != muted.Summary() {
		t.Errorf("muted summary = %q", sums["muted"])
	}
	if want := (grade.Grade{Name: "dark"}).Summary(); sums["dark"] != want {
		t.Errorf("custom dark summary = %q, want %q", sums["dark"], want)
	}

	_, names, _ = namedOptions(nil, motion.Names(), motion.Get,
		func(m motion.Motion) (string, string, string) { return m.Name, m.Description, m.Summary() })
	if !slices.Equal(names, motion.Names()) {
		t.Errorf("motion names = %v", names)
	}
}

func TestOrFirst(t *testing.T) {
	opts := []string{"a", "b"}
	if got := orFirst("b", opts); got != "b" {
		t.Errorf("orFirst(b) = %q", got)
	}
	if got := orFirst("zzz", opts); got != "a" {
		t.Errorf("orFirst(zzz) = %q, want fallback a", got)
	}
}

func TestValidateBPM(t *testing.T) {
	for in, wantErr := range map[string]bool{"": false, "auto": false, "disable": false, "140": false, "0": true, "-3": true, "abc": true, "off": true} {
		if err := validateBPM(in); (err != nil) != wantErr {
			t.Errorf("validateBPM(%q) err = %v, wantErr %v", in, err, wantErr)
		}
	}
}

func TestLayoutOptions(t *testing.T) {
	var got []string
	for _, o := range layoutOptions() {
		got = append(got, o.Value)
	}
	if !slices.Equal(got, []string{LayoutSingle, LayoutGrid, LayoutTrain}) {
		t.Errorf("layouts = %v", got)
	}
}

func TestValidateImagesAndGridN(t *testing.T) {
	if validateImages(nil) == nil || validateImages([]string{"a"}) != nil {
		t.Error("need at least 1 image")
	}
	for in, ok := range map[string]bool{"1": true, "12": true, "0": false, "13": false, "x": false, "": false} {
		if (validateGridN(in) == nil) != ok {
			t.Errorf("validateGridN(%q) ok=%v", in, !ok)
		}
	}
}

func TestAudioOptionsEndWithSilentLoop(t *testing.T) {
	opts := audioOptions([]string{"a.wav", "b.mp3"})
	var got []string
	for _, o := range opts {
		got = append(got, o.Value)
	}
	if !slices.Equal(got, []string{"a.wav", "b.mp3", ""}) {
		t.Errorf("values = %q", got)
	}
}

func TestAfterPreviewOptions(t *testing.T) {
	values := func(reseed bool) []Next {
		var got []Next
		for _, o := range afterPreviewOptions(reseed) {
			got = append(got, o.Value)
		}
		return got
	}
	if got := values(true); !slices.Equal(got, []Next{NextFull, NextLook, NextReseed, NextEverything, NextQuit}) {
		t.Errorf("with reseed = %v", got)
	}
	if got := values(false); slices.Contains(got, NextReseed) || got[0] != NextFull {
		t.Errorf("without reseed = %v", got)
	}
}

func TestValidateOutputDir(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for in, ok := range map[string]bool{"": true, "  ": true, dir: true, " " + dir + " ": true, file: false, filepath.Join(dir, "nope"): false} {
		if (validateOutputDir(in) == nil) != ok {
			t.Errorf("validateOutputDir(%q) ok = %v, want %v", in, !ok, ok)
		}
	}
}

func TestParseBPM(t *testing.T) {
	tests := []struct {
		in             string
		hasAudio       bool
		bpm            int
		auto, disabled bool
	}{
		{"", true, 0, true, false},        // blank detects
		{"auto", true, 0, true, false},    // still accepted
		{"disable", true, 0, false, true}, // no tempo
		{" 140 ", true, 140, false, false},
		{"", false, 0, false, false}, // no audio: runs at 120
		{"disable", false, 0, false, true},
		{"90", false, 90, false, false},
	}
	for _, tt := range tests {
		bpm, auto, disabled := parseBPM(tt.in, tt.hasAudio)
		if bpm != tt.bpm || auto != tt.auto || disabled != tt.disabled {
			t.Errorf("parseBPM(%q, audio=%v) = %d, auto=%v, disabled=%v; want %d, %v, %v",
				tt.in, tt.hasAudio, bpm, auto, disabled, tt.bpm, tt.auto, tt.disabled)
		}
	}
}

func TestBPMFieldRoundTrips(t *testing.T) {
	for _, c := range []struct {
		bpm      int
		disabled bool
		want     string
	}{{0, false, ""}, {140, false, "140"}, {0, true, "disable"}} {
		got := bpmField(c.bpm, c.disabled)
		if got != c.want {
			t.Errorf("bpmField(%d, %v) = %q, want %q", c.bpm, c.disabled, got, c.want)
		}
		if bpm, _, disabled := parseBPM(got, true); bpm != c.bpm || disabled != c.disabled {
			t.Errorf("%q reads back as %d, disabled=%v", got, bpm, disabled)
		}
	}
}
