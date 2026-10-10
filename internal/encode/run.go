package encode

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Progress is a snapshot of a running encode.
type Progress struct {
	Done  time.Duration // how much of the video has been encoded
	Size  int64         // bytes written so far
	Speed float64       // multiple of real time, e.g. 3.2
}

// Run executes ffmpeg with the given parts and calls onProgress as it goes.
// The output file is overwritten if it exists. Cancelling ctx kills ffmpeg;
// the caller is responsible for removing the partial output.
func Run(ctx context.Context, parts []Part, onProgress func(Progress)) error {
	args := append([]string{"-hide_banner", "-nostdin", "-loglevel", "error",
		"-progress", "pipe:1", "-nostats", "-y"}, Args(parts)...)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting ffmpeg: %w", err)
	}

	// ffmpeg prints blocks of key=value lines, each ending with "progress=...".
	var p Progress
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		key, val, _ := strings.Cut(sc.Text(), "=")
		switch key {
		case "out_time_us":
			if us, err := strconv.ParseInt(val, 10, 64); err == nil && us >= 0 {
				p.Done = time.Duration(us) * time.Microsecond
			}
		case "total_size":
			if n, err := strconv.ParseInt(val, 10, 64); err == nil {
				p.Size = n
			}
		case "speed":
			if f, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(val), "x"), 64); err == nil {
				p.Speed = f
			}
		case "progress":
			if onProgress != nil {
				onProgress(p)
			}
		}
	}

	err = cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return fmt.Errorf("ffmpeg: %w", err)
		}
		return errors.New("ffmpeg: " + lastLines(msg, 6))
	}
	return nil
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
