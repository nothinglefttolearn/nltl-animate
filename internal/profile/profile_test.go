package profile

import (
	"slices"
	"testing"
)

func TestProfilesConsistent(t *testing.T) {
	names := Names()
	if !slices.IsSorted(names) {
		t.Errorf("Names() not sorted: %v", names)
	}
	for _, n := range names {
		p, ok := Get(n)
		if !ok {
			t.Fatalf("Get(%q) not found", n)
		}
		if p.Name != n {
			t.Errorf("Profiles[%q].Name = %q; map key and Name must match", n, p.Name)
		}
		if p.Width <= 0 || p.Height <= 0 || p.Width%2 != 0 || p.Height%2 != 0 {
			t.Errorf("%s: %dx%d must be positive and even for yuv420p", n, p.Width, p.Height)
		}
		if p.Suffix == "" {
			t.Errorf("%s: empty Suffix", n)
		}
	}
}
