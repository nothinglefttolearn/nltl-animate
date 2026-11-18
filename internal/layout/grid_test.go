package layout

import (
	"strings"
	"testing"

	"github.com/nothinglefttolearn/nltl-animate/internal/grade"
	"github.com/nothinglefttolearn/nltl-animate/internal/motion"
)

func TestGridFilter(t *testing.T) {
	m, _ := motion.Get("steady")
	g, _ := grade.Get("muted")
	got := GridFilter(m, g, 1080, 1920, 4, 120)

	for _, want := range []string{
		"scale=270:480:force_original_aspect_ratio=increase,crop=270:480,", // motion at tile size
		"s=270x480,",
		"split=16[t0][t1]",
		"[t15];[t0][t1]",
		"[t15]xstack=inputs=16:grid=4x4,scale=1080:1920,setsar=1,",
		"," + g.Filter(m.PulseExpr(120)),
	} {
		if !strings.Contains(got, want) {
			t.Errorf("filter missing %q\n got: %s", want, got)
		}
	}
	if !strings.HasSuffix(got, g.Filter(m.PulseExpr(120))) {
		t.Error("grade must be applied last, over the whole frame")
	}
	if strings.Count(got, "noise=") != 1 || strings.Count(got, "vignette=") != 1 {
		t.Error("grain and vignette must be applied exactly once")
	}
}

func TestGridFilterOddCellsRoundedToEven(t *testing.T) {
	m, _ := motion.Get("gentle")
	g, _ := grade.Get("clean")
	// 1080/7 = 154.3 -> 154; 1920/7 = 274.3 -> 274.
	got := GridFilter(m, g, 1080, 1920, 7, 0)
	if !strings.Contains(got, "crop=154:274") {
		t.Errorf("unexpected tile size: %s", got)
	}
	// 1080/5 = 216 even already; 1080/9 = 120.
	if got := GridFilter(m, g, 1080, 1080, 9, 0); !strings.Contains(got, "crop=120:120") {
		t.Errorf("unexpected tile size: %s", got)
	}
	if even(155) != 154 || even(1) != 2 {
		t.Error("even() rounding wrong")
	}
}

func TestGridFilterN1IsSingle(t *testing.T) {
	m, _ := motion.Get("restless")
	g, _ := grade.Get("dark")
	if GridFilter(m, g, 1080, 1080, 1, 90) != Filter(m, g, 1080, 1080, 90) {
		t.Error("1x1 grid should equal the single-image filter")
	}
}
