package grade

import (
	"slices"
	"strings"
	"testing"
)

func TestNamesSortedAndComplete(t *testing.T) {
	names := Names()
	if !slices.IsSorted(names) || len(names) != len(BuiltIn) {
		t.Fatalf("Names() = %v", names)
	}
	for _, n := range names {
		g, ok := Get(n)
		if !ok || g.Name != n {
			t.Errorf("Get(%q) = %+v, %v; map key and Name must match", n, g, ok)
		}
		if err := g.Validate(); err != nil {
			t.Errorf("built-in %s invalid: %v", n, err)
		}
	}
	if _, ok := Get(Default); !ok {
		t.Errorf("default %q is not a built-in", Default)
	}
	if _, ok := Get("does-not-exist"); ok {
		t.Error("Get returned ok for unknown grade")
	}
}

func TestFilter(t *testing.T) {
	g, _ := Get("muted")
	got := g.Filter("0.02*sin(t)")
	for _, s := range []string{
		"gblur=sigma=0.6:steps=1",
		"eq=brightness='0.02*sin(t)':contrast=1.15:saturation=0.70:eval=frame", // per frame, or the pulse never moves
		"vignette=angle=0.518363", // 33% of PI/2
		"noise=alls=25:allf=t+u",
	} {
		if !strings.Contains(got, s) {
			t.Errorf("filter missing %q\n got: %s", s, got)
		}
	}
}

// Every dial at 0 leaves the image untouched.
func TestZeroIsUntouched(t *testing.T) {
	got := Grade{Name: "none"}.Filter("0")
	want := "gblur=sigma=0.0:steps=1,eq=brightness='0':contrast=1.00:saturation=1.00:eval=frame,vignette=angle=0.000000,noise=alls=0:allf=t+u"
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestSummary(t *testing.T) {
	g, _ := Get("muted")
	if got, want := g.Summary(), "contrast +15, saturation -30, softness 6, vignette 33, grain 25"; got != want {
		t.Errorf("Summary = %q, want %q", got, want)
	}
}

func TestValidate(t *testing.T) {
	base, _ := Get("muted")
	tests := []struct {
		name    string
		mutate  func(*Grade)
		wantErr []string
	}{
		{"valid", func(*Grade) {}, nil},
		{"missing name", func(g *Grade) { g.Name = "" }, []string{"name is required"}},
		{"too high", func(g *Grade) { g.Contrast, g.Saturation, g.Softness, g.Vignette, g.Grain = 101, 101, 101, 101, 101 },
			[]string{"contrast", "saturation", "softness", "vignette", "grain"}},
		{"too low", func(g *Grade) { g.Contrast, g.Saturation, g.Softness, g.Vignette, g.Grain = -101, -101, -1, -1, -1 },
			[]string{"contrast", "saturation", "softness", "vignette", "grain"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := base
			tt.mutate(&g)
			err := g.Validate()
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
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
