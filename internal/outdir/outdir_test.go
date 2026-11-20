package outdir

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	t.Setenv(EnvVar, "/from/env")
	if dir, from := Resolve("/from/flag"); dir != "/from/flag" || from != FlagSource {
		t.Errorf("flag should win: %q, %q", dir, from)
	}
	if dir, from := Resolve(""); dir != "/from/env" || from != EnvVar {
		t.Errorf("env without flag: %q, %q", dir, from)
	}
	t.Setenv(EnvVar, "")
	if dir, from := Resolve(""); dir != "" || from != "" {
		t.Errorf("empty env should count as unset: %q, %q", dir, from)
	}
}

func TestCheck(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if got, err := Check(dir); err != nil || got != dir {
		t.Errorf("Check(existing dir) = %q, %v", got, err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("the write check left files behind: %v", entries)
	}
	for name, c := range map[string]struct{ path, want string }{
		"missing": {filepath.Join(dir, "nope"), "does not exist"},
		"file":    {file, "is a file"},
	} {
		if _, err := Check(c.path); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", name, err, c.want)
		}
	}

	if runtime.GOOS != "windows" && os.Getuid() != 0 {
		ro := filepath.Join(dir, "ro")
		if err := os.Mkdir(ro, 0o500); err != nil {
			t.Fatal(err)
		}
		if _, err := Check(ro); err == nil || !strings.Contains(err.Error(), "not writable") {
			t.Errorf("read-only: err = %v", err)
		}
	}
}

func TestCheckExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, "Videos"), 0o700); err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		"~":        home,
		"~/Videos": filepath.Join(home, "Videos"),
	} {
		if got, err := Check(in); err != nil || got != want {
			t.Errorf("Check(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := Check("~other/x"); err == nil || !strings.Contains(err.Error(), "~other/x") {
		t.Errorf("~user is not expanded and should be reported as is: %v", err)
	}
}
