package interactive

import (
	"slices"
	"testing"

	"github.com/nothinglefttolearn/nltl-clip/internal/preset"
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
	for in, wantErr := range map[string]bool{"": false, "140": false, "0": true, "-3": true, "abc": true} {
		if err := validateBPM(in); (err != nil) != wantErr {
			t.Errorf("validateBPM(%q) err = %v, wantErr %v", in, err, wantErr)
		}
	}
}
