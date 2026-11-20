package main

import (
	"errors"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nothinglefttolearn/nltl-animate/internal/interactive"
	"github.com/nothinglefttolearn/nltl-animate/internal/outdir"
)

// fakeAsk replaces the interactive questions: the form returns first, and
// after each preview the answers come from next in order. It returns the
// output file of every render, and what each preview looked like when it
// was opened.
type fakeAsk struct {
	first   interactive.Choices
	next    []interactive.Next
	look    func(*interactive.Choices)
	again   func(*interactive.Choices)
	opened  []string
	seeds   []uint64
	motions []string
}

func (f *fakeAsk) install(t *testing.T, o *renderOptions) {
	t.Helper()
	oldAll, oldLook, oldNext, oldOpen := askAll, askLook, askNext, openFile
	t.Cleanup(func() { askAll, askLook, askNext, openFile = oldAll, oldLook, oldNext, oldOpen })
	askAll = func(d interactive.Defaults) (*interactive.Choices, error) {
		if d.Previous != nil {
			c := *d.Previous
			f.again(&c)
			return &c, nil
		}
		c := f.first
		return &c, nil
	}
	askLook = func(_ interactive.Defaults, c *interactive.Choices) error {
		f.look(c)
		return nil
	}
	askNext = func(bool) (interactive.Next, error) {
		if len(f.next) == 0 {
			return interactive.NextQuit, errors.New("asked once too often")
		}
		n := f.next[0]
		f.next = f.next[1:]
		return n, nil
	}
	// The fake ffmpeg writes nothing, so "opening" the preview creates it.
	openFile = func(path string) error {
		f.opened = append(f.opened, path)
		f.seeds = append(f.seeds, o.Seed)
		f.motions = append(f.motions, o.MotionName)
		return os.WriteFile(path, []byte("preview"), 0o600)
	}
}

func interactiveSetup(t *testing.T) (dir string, c interactive.Choices) {
	t.Helper()
	fakeFFmpeg(t)
	useTestTime(t)
	dir = t.TempDir()
	audio := filepath.Join(dir, "song.wav")
	red := writePNG(t, dir, "red.png", color.RGBA{R: 255, A: 255})
	blue := writePNG(t, dir, "blue.png", color.RGBA{B: 255, A: 255})
	return dir, interactive.Choices{
		Layout: interactive.LayoutSingle, AudioPath: audio, ImagePaths: []string{red, blue},
		GradeName: "muted", MotionName: "steady", GridN: 3, Tiles: tilesSame, Preview: true,
	}
}

func TestInteractivePreviewThenFull(t *testing.T) {
	dir, c := interactiveSetup(t)
	o := defaultRenderOptions()
	f := &fakeAsk{
		first: c,
		next:  []interactive.Next{interactive.NextLook, interactive.NextReseed, interactive.NextFull},
		look:  func(c *interactive.Choices) { c.MotionName = "pound" },
	}
	f.install(t, &o)
	if err := runInteractive(&o); err != nil {
		t.Fatal(err)
	}
	preview := filepath.Join(dir, interactive.PreviewFile)
	if want := []string{preview, preview, preview}; !slices.Equal(f.opened, want) {
		t.Errorf("opened %v, want the same preview file three times", f.opened)
	}
	if !slices.Equal(f.motions, []string{"steady", "pound", "pound"}) {
		t.Errorf("motions per preview = %v", f.motions)
	}
	if f.seeds[0] != f.seeds[1] || f.seeds[1] == f.seeds[2] {
		t.Errorf("seeds = %v: changing the look keeps the seed, a reseed changes it", f.seeds)
	}
	if _, err := os.Stat(preview); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the preview should be deleted before the full render: %v", err)
	}
}

func TestInteractiveQuitKeepsPreview(t *testing.T) {
	dir, c := interactiveSetup(t)
	o := defaultRenderOptions()
	f := &fakeAsk{first: c, next: []interactive.Next{interactive.NextQuit}}
	f.install(t, &o)
	if err := runInteractive(&o); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, interactive.PreviewFile)); err != nil {
		t.Errorf("quitting should keep the preview: %v", err)
	}
}

func TestInteractivePreviewReplacesLastRun(t *testing.T) {
	dir, c := interactiveSetup(t)
	preview := filepath.Join(dir, interactive.PreviewFile)
	if err := os.WriteFile(preview, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	fakePrompt(t, false, "") // asking would fail: no terminal
	o := defaultRenderOptions()
	f := &fakeAsk{first: c, next: []interactive.Next{interactive.NextQuit}}
	f.install(t, &o)
	if err := runInteractive(&o); err != nil {
		t.Fatalf("an old preview should be replaced without asking: %v", err)
	}
}

func TestInteractiveEverythingAgainWithoutPreview(t *testing.T) {
	dir, c := interactiveSetup(t)
	o := defaultRenderOptions()
	f := &fakeAsk{
		first: c,
		next:  []interactive.Next{interactive.NextEverything},
		again: func(c *interactive.Choices) { c.Layout, c.Preview = interactive.LayoutGrid, false },
	}
	f.install(t, &o)
	if err := runInteractive(&o); err != nil {
		t.Fatal(err)
	}
	if len(f.opened) != 1 {
		t.Errorf("previews = %d, want 1", len(f.opened))
	}
	if _, err := os.Stat(filepath.Join(dir, interactive.PreviewFile)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the preview should be deleted before the full render: %v", err)
	}
}

func TestInteractiveNoPreviewRendersFull(t *testing.T) {
	_, c := interactiveSetup(t)
	c.Preview = false
	o := defaultRenderOptions()
	f := &fakeAsk{first: c}
	f.install(t, &o)
	if err := runInteractive(&o); err != nil {
		t.Fatal(err)
	}
	if len(f.opened) != 0 {
		t.Errorf("no preview asked for, but opened %v", f.opened)
	}
}

func TestOutputDirSource(t *testing.T) {
	t.Setenv(outdir.EnvVar, "/env")
	for _, c := range []struct{ dir, flag, want string }{
		{"", "", formSource},
		{"/env", "", outdir.EnvVar},
		{"/env", "/flag", outdir.FlagSource},
		{"/typed", "", outdir.FlagSource},
	} {
		if got := outputDirSource(c.dir, c.flag); got != c.want {
			t.Errorf("outputDirSource(%q, %q) = %q, want %q", c.dir, c.flag, got, c.want)
		}
	}
}

// A field left blank in the form writes next to the input, even with the
// variable set.
func TestBlankFormOutputDirIgnoresVariable(t *testing.T) {
	t.Setenv(outdir.EnvVar, t.TempDir())
	o := opts(layoutSingle, "i")
	o.AudioPath, o.outputDirFrom = "/music/song.wav", formSource
	if err := o.resolveOutputDir(); err != nil {
		t.Fatal(err)
	}
	if got := filepath.Dir(o.outputPath(testTime)); got != "/music" {
		t.Errorf("output dir = %s, want /music", got)
	}
}
