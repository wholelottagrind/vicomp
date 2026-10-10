// Package encode turns human-friendly settings into an ffmpeg command,
// explains every knob, and runs the encode.
package encode

import (
	"fmt"
	"slices"

	"github.com/wholelottagrind/vicomp/internal/media"
)

// Settings is everything that affects the ffmpeg command.
type Settings struct {
	Codec   string `toml:"codec"`   // "h264" or "h265"
	Encoder string `toml:"encoder"` // "software" or "hardware"
	Quality int    `toml:"quality"` // CRF scale: lower = better and bigger
	Speed   string `toml:"speed"`   // x264/x265 preset name
	MaxRes  int    `toml:"max_res"` // cap for the short side in px, 0 = keep original
	MaxFPS  int    `toml:"max_fps"` // 0 = keep original
	Audio   string `toml:"audio"`   // AAC bitrate like "128k", or "none"
}

var (
	codecs     = []string{"h264", "h265"}
	encoders   = []string{"software", "hardware"}
	qualities  = []int{18, 20, 23, 26, 28, 30, 33}
	speeds     = []string{"veryfast", "faster", "fast", "medium", "slow", "slower", "veryslow"}
	resolution = []int{0, 2160, 1440, 1080, 720, 480}
	fpsCaps    = []int{0, 60, 30, 24, 15}
	audios     = []string{"128k", "96k", "64k", "none"}
)

// Validate reports settings that vicomp doesn't know how to use
// (e.g. a typo in a preset in the config file).
func (s Settings) Validate() error {
	switch {
	case !slices.Contains(codecs, s.Codec):
		return fmt.Errorf("codec %q: must be one of %v", s.Codec, codecs)
	case !slices.Contains(encoders, s.Encoder):
		return fmt.Errorf("encoder %q: must be one of %v", s.Encoder, encoders)
	case !slices.Contains(qualities, s.Quality):
		return fmt.Errorf("quality %d: must be one of %v", s.Quality, qualities)
	case !slices.Contains(speeds, s.Speed):
		return fmt.Errorf("speed %q: must be one of %v", s.Speed, speeds)
	case !slices.Contains(resolution, s.MaxRes):
		return fmt.Errorf("max_res %d: must be one of %v", s.MaxRes, resolution)
	case !slices.Contains(fpsCaps, s.MaxFPS):
		return fmt.Errorf("max_fps %d: must be one of %v", s.MaxFPS, fpsCaps)
	case !slices.Contains(audios, s.Audio):
		return fmt.Errorf("audio %q: must be one of %v", s.Audio, audios)
	}
	return nil
}

// Field is one adjustable setting, with everything the UI needs to show
// and explain it.
type Field struct {
	Name  string
	About string // what this setting does, in plain words
	// Value is the current value as shown to the user.
	Value func(s Settings) string
	// Note explains the current value, using the source video for context.
	Note func(s Settings, in media.Info) string
	// Step moves the value one option left (-1) or right (+1).
	Step func(s *Settings, dir int)
}

// step moves cur one position in vals, stopping at the ends.
func step[T comparable](vals []T, cur T, dir int) T {
	i := slices.Index(vals, cur) + dir
	return vals[max(0, min(len(vals)-1, i))]
}

// Fields lists the settings in the order they're shown.
var Fields = []Field{
	{
		Name: "Codec",
		About: "The video format. H.264 is the safe choice that plays everywhere. " +
			"H.265 (HEVC) is newer and makes smaller files at the same quality, " +
			"but some older phones and apps can't play it.",
		Value: func(s Settings) string { return map[string]string{"h264": "H.264", "h265": "H.265"}[s.Codec] },
		Note: func(s Settings, _ media.Info) string {
			if s.Codec == "h265" {
				return "smaller file, but some older devices can't preview it inline"
			}
			return "plays inline in Telegram on every device"
		},
		Step: func(s *Settings, d int) { s.Codec = step(codecs, s.Codec, d) },
	},
	{
		Name: "Encoder",
		About: "What does the compressing. Software (x264/x265) runs on the CPU and gives the best " +
			"quality per megabyte. Hardware uses your Mac's built-in video engine (VideoToolbox). " +
			"It's much faster, but files come out bigger at the same quality.",
		Value: func(s Settings) string {
			if s.Encoder == "hardware" {
				return "hardware"
			}
			return "software"
		},
		Note: func(s Settings, _ media.Info) string {
			if s.Encoder == "hardware" {
				return "very fast, file somewhat bigger"
			}
			return "slower, smallest file for the quality"
		},
		Step: func(s *Settings, d int) { s.Encoder = step(encoders, s.Encoder, d) },
	},
	{
		Name: "Quality",
		About: "How good the picture looks. This is x264's CRF scale: LOWER = sharper and " +
			"bigger. Every +6 roughly halves the file size. ffmpeg's default is 23. " +
			"Screen recordings still look fine at 26–28 because they're mostly flat UI.",
		Value: func(s Settings) string { return fmt.Sprint(s.Quality) },
		Note: func(s Settings, _ media.Info) string {
			note := map[int]string{
				18: "visually lossless, big file",
				20: "very high, hard to tell apart from the original",
				23: "ffmpeg's default (what your old command used)",
				26: "great for screen recordings: text stays crisp",
				28: "a bit softer, fine for quick 'look at this' clips",
				30: "small; tiny text may get blurry",
				33: "very small; blocky, only when size really matters",
			}[s.Quality]
			if s.Encoder == "hardware" {
				note += fmt.Sprintf(" (hardware: -q:v %d)", hardwareQuality(s.Quality))
			}
			return note
		},
		Step: func(s *Settings, d int) { s.Quality = step(qualities, s.Quality, d) },
	},
	{
		Name: "Speed",
		About: "How hard the encoder works to shrink the file. Slower = smaller file at the " +
			"same quality. It doesn't change how the video looks, only the size and how " +
			"long you wait. Hardware encoding ignores this.",
		Value: func(s Settings) string { return s.Speed },
		Note: func(s Settings, _ media.Info) string {
			if s.Encoder == "hardware" {
				return "not used by the hardware encoder"
			}
			return map[string]string{
				"veryfast": "quick encode, noticeably bigger file",
				"faster":   "fast, bigger file",
				"fast":     "a bit faster than medium, a bit bigger",
				"medium":   "x264's default (what your old command used)",
				"slow":     "good tradeoff: slower, a bit smaller",
				"slower":   "takes a while, slightly smaller again",
				"veryslow": "very slow, tiny extra savings",
			}[s.Speed]
		},
		Step: func(s *Settings, d int) { s.Speed = step(speeds, s.Speed, d) },
	},
	{
		Name: "Resolution",
		About: "Limits the picture size (by the short side, so 1080p = 1080 pixels tall for a " +
			"landscape video). Retina recordings are huge, e.g. 2880×1800. On a phone " +
			"1080p looks the same. This is usually the biggest size win. Videos are never upscaled.",
		Value: func(s Settings) string {
			if s.MaxRes == 0 {
				return "original"
			}
			return fmt.Sprintf("%dp", s.MaxRes)
		},
		Note: func(s Settings, in media.Info) string {
			if in.Width == 0 {
				return ""
			}
			w, h, scaled := ScaledSize(in, s.MaxRes)
			if !scaled {
				return fmt.Sprintf("keeps %d×%d", in.Width, in.Height)
			}
			return fmt.Sprintf("%d×%d → %d×%d", in.Width, in.Height, w, h)
		},
		Step: func(s *Settings, d int) { s.MaxRes = step(resolution, s.MaxRes, d) },
	},
	{
		Name: "Frame rate",
		About: "Frames per second. Screen recordings are often 60 fps. For UI demos and " +
			"scrolling, 30 looks almost the same and is smaller. 15 is fine for mostly " +
			"static stuff. The frame rate is only ever lowered, never raised.",
		Value: func(s Settings) string {
			if s.MaxFPS == 0 {
				return "original"
			}
			return fmt.Sprintf("%d fps", s.MaxFPS)
		},
		Note: func(s Settings, in media.Info) string {
			switch {
			case in.FPS == 0:
				return ""
			case !needsFPSCap(in, s.MaxFPS):
				return fmt.Sprintf("keeps %.0f fps", in.FPS)
			default:
				return fmt.Sprintf("%.0f → %d fps", in.FPS, s.MaxFPS)
			}
		},
		Step: func(s *Settings, d int) { s.MaxFPS = step(fpsCaps, s.MaxFPS, d) },
	},
	{
		Name: "Audio",
		About: "The sound track, converted to AAC (the format every player supports). Speech " +
			"sounds fine at 64k. macOS screen recordings usually have no sound at all " +
			"unless you turned on the microphone.",
		Value: func(s Settings) string {
			if s.Audio == "none" {
				return "remove"
			}
			return "AAC " + s.Audio
		},
		Note: func(s Settings, in media.Info) string {
			if !in.HasAudio && in.Width != 0 {
				return "this video has no sound, so this doesn't matter"
			}
			return map[string]string{
				"128k": "good for music and voice",
				"96k":  "fine for voice, saves a little",
				"64k":  "voice only, smallest",
				"none": "drops the sound completely",
			}[s.Audio]
		},
		Step: func(s *Settings, d int) { s.Audio = step(audios, s.Audio, d) },
	},
}

// ScaledSize returns the output dimensions for a resolution cap and whether
// scaling happens at all.
func ScaledSize(in media.Info, maxRes int) (w, h int, scaled bool) {
	short := min(in.Width, in.Height)
	if maxRes == 0 || short <= maxRes {
		return in.Width, in.Height, false
	}
	even := func(x float64) int { return int(x/2+0.5) * 2 }
	if in.Height <= in.Width {
		return even(float64(in.Width) * float64(maxRes) / float64(in.Height)), maxRes, true
	}
	return maxRes, even(float64(in.Height) * float64(maxRes) / float64(in.Width)), true
}

// needsFPSCap reports whether the fps filter should be applied. When the
// source frame rate is unknown, the cap is applied to be safe.
func needsFPSCap(in media.Info, maxFPS int) bool {
	return maxFPS != 0 && (in.FPS == 0 || in.FPS > float64(maxFPS)+0.5)
}

// hardwareQuality maps the CRF-style level to VideoToolbox's -q:v scale
// (1–100, higher = better). It's an approximation: the two encoders
// don't share a scale.
func hardwareQuality(crf int) int {
	return map[int]int{18: 80, 20: 75, 23: 65, 26: 55, 28: 50, 30: 45, 33: 38}[crf]
}
