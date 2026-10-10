package encode

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wholelottagrind/vicomp/internal/media"
)

// Part is a piece of the ffmpeg command together with why it's there.
type Part struct {
	Args []string
	Why  string
}

// Build returns the ffmpeg command (without the leading "ffmpeg") split into
// explained parts.
func Build(s Settings, in media.Info, inPath, outPath string) []Part {
	parts := []Part{{[]string{"-i", inPath}, "the input file"}}

	hw := s.Encoder == "hardware"
	switch {
	case s.Codec == "h264" && !hw:
		parts = append(parts, Part{[]string{"-c:v", "libx264"}, "re-encode the video as H.264 using x264 (software encoder)"})
	case s.Codec == "h265" && !hw:
		parts = append(parts, Part{[]string{"-c:v", "libx265"}, "re-encode the video as H.265 using x265 (software encoder)"})
	case s.Codec == "h264":
		parts = append(parts, Part{[]string{"-c:v", "h264_videotoolbox"}, "re-encode the video as H.264 using the Mac's hardware encoder"})
	default:
		parts = append(parts, Part{[]string{"-c:v", "hevc_videotoolbox"}, "re-encode the video as H.265 using the Mac's hardware encoder"})
	}

	if hw {
		parts = append(parts, Part{[]string{"-q:v", fmt.Sprint(hardwareQuality(s.Quality))},
			"hardware quality, 1–100, higher = better and bigger"})
	} else {
		parts = append(parts,
			Part{[]string{"-crf", fmt.Sprint(s.Quality)}, "constant quality: lower = better and bigger (default 23)"},
			Part{[]string{"-preset", s.Speed}, "encoder effort: slower = smaller file at the same quality (default medium)"})
		if s.Codec == "h265" {
			parts = append(parts, Part{[]string{"-x265-params", "log-level=error"}, "keep x265 from printing its own logs"})
		}
	}
	if s.Codec == "h265" {
		parts = append(parts, Part{[]string{"-tag:v", "hvc1"}, "label the H.265 stream so Apple devices and QuickTime will play it"})
	}

	var filters, why []string
	if w, h, ok := ScaledSize(in, s.MaxRes); ok {
		if in.Height <= in.Width {
			filters = append(filters, fmt.Sprintf("scale=-2:%d", h))
		} else {
			filters = append(filters, fmt.Sprintf("scale=%d:-2", w))
		}
		why = append(why, fmt.Sprintf("scale to %d×%d (-2 = keep the aspect ratio, round to an even number)", w, h))
	}
	if needsFPSCap(in, s.MaxFPS) {
		filters = append(filters, fmt.Sprintf("fps=%d", s.MaxFPS))
		why = append(why, fmt.Sprintf("drop frames down to %d per second", s.MaxFPS))
	}
	if len(filters) > 0 {
		parts = append(parts, Part{[]string{"-vf", strings.Join(filters, ",")}, "video filters: " + strings.Join(why, "; ")})
	}

	parts = append(parts, Part{[]string{"-pix_fmt", "yuv420p"}, "the standard color format; some players can't play anything else"})

	if s.Audio == "none" {
		parts = append(parts, Part{[]string{"-an"}, "no audio"})
	} else {
		parts = append(parts, Part{[]string{"-c:a", "aac", "-b:a", s.Audio}, "re-encode the audio as AAC at " + s.Audio + "bit/s"})
	}

	parts = append(parts,
		Part{[]string{"-movflags", "+faststart"}, "put the index at the start of the file so playback can begin before the download finishes"},
		Part{[]string{outPath}, "the output file; the .mp4 extension picks the MP4 container"})
	return parts
}

// Args flattens parts into an argument list.
func Args(parts []Part) []string {
	var args []string
	for _, p := range parts {
		args = append(args, p.Args...)
	}
	return args
}

// CommandLine renders the command the way you'd type it in a shell.
// Paths are shortened to their base name when short is true.
func CommandLine(parts []Part, short bool) string {
	words := []string{"ffmpeg"}
	for i, p := range parts {
		for j, a := range p.Args {
			isPath := (i == 0 && j == 1) || i == len(parts)-1
			if short && isPath {
				a = filepath.Base(a)
			}
			words = append(words, ShellQuote(a))
		}
	}
	return strings.Join(words, " ")
}

// ShellQuote quotes s for zsh/bash if needed.
func ShellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n'\"\\$`!*?[](){}<>|&;#~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
