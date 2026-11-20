// Package outdir decides and checks the directory a clip is written to:
// --output-dir, then the NLTL_ANIMATE_OUTPUT_DIR environment variable, then
// (decided by the caller) next to the input.
package outdir

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EnvVar names the environment variable that sets the output directory
// when --output-dir is not given.
const EnvVar = "NLTL_ANIMATE_OUTPUT_DIR"

// FlagSource is the source of a directory given with --output-dir.
const FlagSource = "--output-dir"

// Resolve picks the output directory: flag when set, else the environment
// variable when set and not empty. It returns "" when neither is set. from
// names where the directory came from, for error messages.
func Resolve(flag string) (dir, from string) {
	if flag != "" {
		return flag, FlagSource
	}
	if env := os.Getenv(EnvVar); env != "" {
		return env, EnvVar
	}
	return "", ""
}

// Check expands a leading ~ in dir and reports whether it is a directory
// that can be written to. It returns the expanded path.
func Check(dir string) (string, error) {
	dir, err := expandHome(dir)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "", fmt.Errorf("output directory %s does not exist", dir)
	case err != nil:
		return "", fmt.Errorf("output directory %s: %w", dir, err)
	case !fi.IsDir():
		return "", fmt.Errorf("output directory %s is a file, not a directory", dir)
	}
	f, err := os.CreateTemp(dir, ".nltl-animate-write-check-*")
	if err != nil {
		return "", fmt.Errorf("output directory %s is not writable", dir)
	}
	_ = f.Close()
	_ = os.Remove(f.Name())
	return dir, nil
}

// expandHome replaces a leading ~ (alone or followed by a separator) with
// the home directory. A quoted "~/Videos" in a shell profile is not
// expanded by the shell, so it reaches us as is.
func expandHome(dir string) (string, error) {
	if dir != "~" && !strings.HasPrefix(dir, "~/") && !strings.HasPrefix(dir, "~"+string(filepath.Separator)) {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expanding ~ in output directory %s: %w", dir, err)
	}
	return filepath.Join(home, dir[1:]), nil
}
