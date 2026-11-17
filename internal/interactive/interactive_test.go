package interactive

import (
	"slices"
	"testing"

	"github.com/nothinglefttolearn/nltl-animate/internal/preset"
)

func TestPresetOptions(t *testing.T) {
	custom := []preset.Preset{{Name: "mine"}, {Name: "dark"}, {Name: "mine"}}
	opts, names := presetOptions(custom)

	want := []string{"mine", "dark", "clean", "industrial"}
	if !slices.Equal(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
	if len(opts) != len(names) {
		t.Errorf("got %d options for %d names", len(opts), len(names))
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

func TestStyleOptions(t *testing.T) {
	var got []string
	for _, o := range styleOptions() {
		got = append(got, o.Value)
	}
	if !slices.Equal(got, []string{StyleSingle, StyleGrid, StyleTrain}) {
		t.Errorf("styles = %v", got)
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
