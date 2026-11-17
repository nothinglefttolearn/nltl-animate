package ffmpeg

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nothinglefttolearn/nltl-animate/internal/video"
)

// stderrTailLines caps how much tool output is copied into an error message.
const stderrTailLines = 20

// FrameSource produces raw RGBA frames (4 bytes per pixel, no row padding)
// that are piped to ffmpeg's stdin instead of looping a still image.
type FrameSource interface {
	// Size is the width and height of every frame.
	Size() (width, height int)
	// WriteFrames writes exactly n frames to w, in order.
	WriteFrames(w io.Writer, n int) error
}

type Options struct {
	// ImagePath is a still image looped for the whole clip. It is ignored
	// when Frames is set.
	ImagePath string
	// Frames, when set, supplies every video frame at video.FPS.
	Frames FrameSource

	AudioPath  string
	OutputPath string
	// Filter is a filtergraph applied to the video input. It may contain
	// several ';'-separated chains (e.g. split/xstack) but must have exactly
	// one unlabelled input and output; Render adds the [0:v] and [v] labels.
	Filter   string
	Duration float64
	Preview  bool
	Verbose  bool
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

// DecodeMono decodes up to maxSeconds of audio from path as mono 32-bit
// float samples at rate Hz.
func DecodeMono(path string, rate int, maxSeconds float64) ([]float32, error) {
	cmd := exec.Command("ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-i", path,
		"-t", fmt.Sprintf("%.3f", maxSeconds),
		"-vn", "-ac", "1", "-ar", strconv.Itoa(rate),
		"-f", "f32le", "pipe:1",
	)
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			stderr = exitErr.Stderr
		}
		return nil, commandError("ffmpeg", err, stderr)
	}
	samples := make([]float32, len(out)/4)
	for i := range samples {
		samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(out[4*i:]))
	}
	return samples, nil
}

func Render(opts Options) error {
	duration := opts.Duration
	if opts.Preview && duration > video.PreviewSeconds {
		duration = video.PreviewSeconds
	}

	cmd := exec.Command("ffmpeg", buildArgs(opts, duration)...)

	run := cmd.Run
	if opts.Frames != nil {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("ffmpeg stdin: %w", err)
		}
		run = func() error { return runWithFrames(cmd, stdin, opts.Frames, video.FrameCount(duration)) }
	}

	if opts.Verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := run(); err != nil {
			return commandError("ffmpeg", err, nil) // output already shown
		}
		return nil
	}

	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	if err := runWithSpinner("Rendering", run); err != nil {
		return commandError("ffmpeg", err, stderr.Bytes())
	}
	return nil
}

func buildArgs(opts Options, duration float64) []string {
	args := []string{"-y", "-hide_banner"}
	if !opts.Verbose {
		// Only errors on stderr, so what we capture is the useful part.
		args = append(args, "-loglevel", "error")
	}

	if opts.Frames != nil {
		w, h := opts.Frames.Size()
		args = append(args,
			"-f", "rawvideo",
			"-pixel_format", "rgba",
			"-video_size", fmt.Sprintf("%dx%d", w, h),
			"-framerate", strconv.Itoa(video.FPS),
			"-i", "pipe:0",
		)
	} else {
		args = append(args, "-loop", "1", "-i", opts.ImagePath)
	}

	args = append(args,
		"-i", opts.AudioPath,
		"-filter_complex", "[0:v]"+opts.Filter+"[v]",
		"-map", "[v]",
		"-map", "1:a",
		"-c:v", "libx264",
	)

	switch {
	case opts.Preview:
		args = append(args, "-preset", "ultrafast", "-crf", "35")
	case opts.Frames == nil:
		args = append(args, "-tune", "stillimage", "-crf", "18")
	default:
		args = append(args, "-crf", "18")
	}

	return append(args,
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "192k",
		"-r", strconv.Itoa(video.FPS),
		"-t", fmt.Sprintf("%.3f", duration),
		"-shortest",
		opts.OutputPath,
	)
}

// runWithFrames starts ffmpeg, streams n frames into its stdin and waits.
// ffmpeg's own failure takes precedence over the write error it causes
// (a broken pipe), and a broken pipe after ffmpeg succeeded (it stopped
// reading because of -t/-shortest) is not an error.
func runWithFrames(cmd *exec.Cmd, stdin io.WriteCloser, src FrameSource, n int) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	writeErr := make(chan error, 1)
	go func() {
		err := src.WriteFrames(stdin, n)
		if cerr := stdin.Close(); err == nil {
			err = cerr
		}
		writeErr <- err
	}()

	waitErr := cmd.Wait()
	werr := <-writeErr
	if waitErr != nil {
		return waitErr
	}
	if werr != nil && !isBrokenPipe(werr) {
		return fmt.Errorf("writing frames: %w", werr)
	}
	return nil
}

func isBrokenPipe(err error) bool {
	return errors.Is(err, syscall.EPIPE) || errors.Is(err, os.ErrClosed) || errors.Is(err, io.ErrClosedPipe)
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
