// Package media finds video files and reads their properties with ffprobe.
package media

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// File is a video found in one of the source directories.
type File struct {
	Path    string
	Name    string
	Size    int64
	ModTime time.Time
}

// Info is what ffprobe tells us about a video.
type Info struct {
	Duration   time.Duration
	Width      int // as displayed (rotation already applied)
	Height     int
	FPS        float64 // average frame rate; 0 if unknown
	VideoCodec string
	HasAudio   bool
}

// Scan lists video files (top level only) in dirs, newest first.
// Missing directories are skipped and reported in the returned warnings.
func Scan(dirs, exts []string) (files []File, warnings []string) {
	seen := map[string]bool{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("can't read %s: %v", dir, err))
			continue
		}
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") || !hasExt(e.Name(), exts) {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if seen[path] {
				continue
			}
			seen[path] = true
			fi, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, File{Path: path, Name: e.Name(), Size: fi.Size(), ModTime: fi.ModTime()})
		}
	}
	slices.SortFunc(files, func(a, b File) int { return b.ModTime.Compare(a.ModTime) })
	return files, warnings
}

func hasExt(name string, exts []string) bool {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	for _, e := range exts {
		if strings.EqualFold(strings.TrimPrefix(e, "."), ext) {
			return true
		}
	}
	return false
}

type probeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		AvgFrameRate string `json:"avg_frame_rate"`
		SideDataList []struct {
			Rotation float64 `json:"rotation"`
		} `json:"side_data_list"`
	} `json:"streams"`
}

// Probe runs ffprobe on path.
func Probe(path string) (Info, error) {
	out, err := exec.Command("ffprobe", "-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", path).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return Info{}, fmt.Errorf("ffprobe: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return Info{}, fmt.Errorf("ffprobe: %w", err)
	}
	var p probeOutput
	if err := json.Unmarshal(out, &p); err != nil {
		return Info{}, fmt.Errorf("ffprobe output: %w", err)
	}

	var info Info
	if d, err := strconv.ParseFloat(p.Format.Duration, 64); err == nil {
		info.Duration = time.Duration(d * float64(time.Second))
	}
	foundVideo := false
	for _, s := range p.Streams {
		switch s.CodecType {
		case "video":
			if foundVideo || s.Width == 0 {
				continue // skip cover art and extra video tracks
			}
			foundVideo = true
			info.VideoCodec = s.CodecName
			info.Width, info.Height = s.Width, s.Height
			for _, sd := range s.SideDataList {
				if r := math.Abs(sd.Rotation); r == 90 || r == 270 {
					info.Width, info.Height = info.Height, info.Width
				}
			}
			info.FPS = parseRate(s.AvgFrameRate)
		case "audio":
			info.HasAudio = true
		}
	}
	if !foundVideo {
		return info, fmt.Errorf("no video stream found")
	}
	return info, nil
}

func parseRate(r string) float64 {
	num, den, ok := strings.Cut(r, "/")
	if !ok {
		f, _ := strconv.ParseFloat(r, 64)
		return f
	}
	n, err1 := strconv.ParseFloat(num, 64)
	d, err2 := strconv.ParseFloat(den, 64)
	if err1 != nil || err2 != nil || d == 0 {
		return 0
	}
	return n / d
}

// HumanSize formats a byte count like "412 MB".
func HumanSize(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 10<<20:
		return fmt.Sprintf("%.0f MB", float64(b)/(1<<20))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(b)/(1<<10))
	}
	return fmt.Sprintf("%d B", b)
}

// HumanDuration formats a duration like "1:48" or "1:02:03".
func HumanDuration(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// HumanAge formats how long ago t was, like "5m ago" or "3d ago".
func HumanAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Format("Jan 2")
}

// Summary is a one-line description like "2880×1800 · 60 fps · 1:48 · h264 · no audio".
func (i Info) Summary() string {
	parts := []string{fmt.Sprintf("%d×%d", i.Width, i.Height)}
	if i.FPS > 0 {
		parts = append(parts, fmt.Sprintf("%.0f fps", i.FPS))
	}
	parts = append(parts, HumanDuration(i.Duration), i.VideoCodec)
	if !i.HasAudio {
		parts = append(parts, "no audio")
	}
	return strings.Join(parts, " · ")
}
