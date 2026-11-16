package ffmpeg

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// stderrTailLines caps how much tool output is copied into an error message.
const stderrTailLines = 20

type Options struct {
	ImagePath  string
	AudioPath  string
	OutputPath string
	Filter     string
	Duration   float64
	Preview    bool
	Verbose    bool
}

func GetDuration(audioPath string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		audioPath,
	)
	// Output() captures stderr into the returned *exec.ExitError.
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			stderr = exitErr.Stderr
		}
		return 0, commandError("ffprobe", err, stderr)
	}
	s := strings.TrimSpace(string(out))
	d, err := strconv.ParseFloat(s, 64)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("ffprobe: could not read a duration from %s (got %q)", audioPath, s)
	}
	return d, nil
}

func Render(opts Options) error {
	duration := opts.Duration
	if opts.Preview && duration > 10 {
		duration = 10
	}

	args := []string{"-y", "-hide_banner"}
	if !opts.Verbose {
		// Only errors on stderr, so what we capture is the useful part.
		args = append(args, "-loglevel", "error")
	}
	args = append(args,
		"-loop", "1",
		"-i", opts.ImagePath,
		"-i", opts.AudioPath,
		"-vf", opts.Filter,
		"-c:v", "libx264",
	)

	if opts.Preview {
		args = append(args, "-preset", "ultrafast", "-crf", "35")
	} else {
		args = append(args, "-tune", "stillimage", "-crf", "18")
	}

	args = append(args,
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "192k",
		"-r", "30",
		"-t", fmt.Sprintf("%.3f", duration),
		"-shortest",
		opts.OutputPath,
	)

	cmd := exec.Command("ffmpeg", args...)

	if opts.Verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return commandError("ffmpeg", err, nil) // output already shown
		}
		return nil
	}

	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := runWithSpinner("Rendering", cmd.Run); err != nil {
		return commandError("ffmpeg", err, stderr.Bytes())
	}
	return nil
}

// commandError wraps a failed tool invocation with the last lines of its
// stderr, and turns a missing binary into an actionable message.
func commandError(name string, err error, stderr []byte) error {
	if errors.Is(err, exec.ErrNotFound) {
		return fmt.Errorf("%s not found in PATH — install ffmpeg (which includes ffprobe): %w", name, err)
	}
	tail := lastLines(strings.TrimSpace(string(stderr)), stderrTailLines)
	if tail == "" {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return fmt.Errorf("%s failed: %w\n%s", name, err, indent(tail, "  "))
}

func lastLines(s string, n int) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = append([]string{"..."}, lines[len(lines)-n:]...)
	}
	return strings.Join(lines, "\n")
}

func indent(s, prefix string) string {
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}

func runWithSpinner(title string, fn func() error) error {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	done := make(chan error, 1)
	go func() { done <- fn() }()

	i := 0
	line := "  " + title
	for {
		select {
		case err := <-done:
			fmt.Printf("\r%s\r", strings.Repeat(" ", len(line)+4))
			return err
		default:
			fmt.Printf("\r%s %s", frames[i%len(frames)], line)
			i++
			time.Sleep(80 * time.Millisecond)
		}
	}
}
