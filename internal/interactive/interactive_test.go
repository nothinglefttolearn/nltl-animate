package interactive

import (
	"slices"
	"testing"

	"github.com/nothinglefttolearn/nltl-animate/internal/grade"
	"github.com/nothinglefttolearn/nltl-animate/internal/motion"
)

func TestNamedOptions(t *testing.T) {
	custom := []grade.Grade{{Name: "mine"}, {Name: "dark"}, {Name: "mine"}}
	opts, names := namedOptions(custom, grade.Names(), grade.Get,
		func(g grade.Grade) (string, string) { return g.Name, g.Description })

	// Custom first, then the built-ins not shadowed by a custom one.
	want := append([]string{"mine", "dark"}, slices.DeleteFunc(grade.Names(), func(n string) bool { return n == "dark" })...)
	if !slices.Equal(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
	if len(opts) != len(names) {
		t.Errorf("got %d options for %d names", len(opts), len(names))
	}

	_, names = namedOptions(nil, motion.Names(), motion.Get,
		func(m motion.Motion) (string, string) { return m.Name, m.Description })
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
	for in, wantErr := range map[string]bool{"": false, "auto": false, "140": false, "0": true, "-3": true, "abc": true} {
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
