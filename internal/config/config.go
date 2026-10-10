// Package config loads and saves ~/.config/vicomp/config.toml.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/wholelottagrind/vicomp/internal/encode"
)

// Config is the user's configuration.
type Config struct {
	SourceDirs       []string                   `toml:"source_dirs"`
	OutputDir        string                     `toml:"output_dir"`
	FilenameTemplate string                     `toml:"filename_template"`
	DefaultPreset    string                     `toml:"default_preset"`
	CopyToClipboard  bool                       `toml:"copy_to_clipboard"`
	RevealInFinder   bool                       `toml:"reveal_in_finder"`
	TrashOriginal    bool                       `toml:"trash_original"`
	Extensions       []string                   `toml:"extensions"`
	Presets          map[string]encode.Settings `toml:"presets"`
}

// Default returns the configuration used on first run.
func Default() Config {
	return Config{
		SourceDirs:       []string{"~/Desktop/Screenshots"},
		OutputDir:        "",
		FilenameTemplate: "{name}-tg",
		DefaultPreset:    "telegram",
		CopyToClipboard:  true,
		Extensions:       []string{"mov", "mp4", "m4v", "mkv", "webm"},
	}
}

// Path returns where the config file lives.
func Path() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "vicomp", "config.toml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "vicomp", "config.toml")
}

// Load reads the config file, creating it with defaults if it doesn't exist.
func Load() (Config, error) {
	cfg := Default()
	path := Path()
	md, err := toml.DecodeFile(path, &cfg)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, Save(cfg)
	}
	if err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	if extra := md.Undecoded(); len(extra) > 0 {
		keys := make([]string, len(extra))
		for i, k := range extra {
			keys[i] = k.String()
		}
		return cfg, fmt.Errorf("%s: unknown setting(s): %s", path, strings.Join(keys, ", "))
	}
	for name, s := range cfg.Presets {
		if err := s.Validate(); err != nil {
			return cfg, fmt.Errorf("%s: preset %q: %w", path, name, err)
		}
	}
	if _, ok := cfg.Preset(cfg.DefaultPreset); !ok {
		return cfg, fmt.Errorf("%s: default_preset %q doesn't exist", path, cfg.DefaultPreset)
	}
	return cfg, nil
}

const header = `# vicomp configuration.
# Edit freely. vicomp also rewrites this file when you save a preset or
# set a default output folder from the app, so comments you add will be lost.

# Folders to look for videos in (top level only). ~ means your home folder.
source_dirs = %s

# Where compressed videos go. Empty = next to the original.
output_dir = %s

# Output filename without extension. Available: {name} (original name),
# {preset} (preset name), {date} (today, YYYY-MM-DD). ".mp4" is appended.
filename_template = %s

# Preset used when the app opens and by "vicomp last".
# Built-in: telegram, tiny, quality, fast, classic, or one of yours below.
default_preset = %s

# After compressing: copy the file to the clipboard (paste into Telegram with ⌘V),
# reveal it in Finder, and/or move the original to the Trash.
copy_to_clipboard = %t
reveal_in_finder = %t
trash_original = %t

# File extensions treated as videos.
extensions = %s
`

const presetsHelp = `
# Your presets. Saved from the app with "s", or written by hand:
#
# [presets.mine]
# codec = "h264"        # h264 | h265
# encoder = "software"  # software | hardware
# quality = 26          # 18 20 23 26 28 30 33 (lower = better and bigger)
# speed = "slow"        # veryfast faster fast medium slow slower veryslow
# max_res = 1080        # 0 (original) 2160 1440 1080 720 480
# max_fps = 30          # 0 (original) 60 30 24 15
# audio = "128k"        # 128k 96k 64k none
`

// Save writes cfg to the config file.
func Save(cfg Config) error {
	var b bytes.Buffer
	fmt.Fprintf(&b, header, tomlList(cfg.SourceDirs), tomlString(cfg.OutputDir), tomlString(cfg.FilenameTemplate),
		tomlString(cfg.DefaultPreset), cfg.CopyToClipboard, cfg.RevealInFinder, cfg.TrashOriginal, tomlList(cfg.Extensions))
	b.WriteString(presetsHelp)
	if len(cfg.Presets) > 0 {
		b.WriteString("\n")
		err := toml.NewEncoder(&b).Encode(struct {
			Presets map[string]encode.Settings `toml:"presets"`
		}{cfg.Presets})
		if err != nil {
			return err
		}
	}
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func tomlList(ss []string) string {
	q := make([]string, len(ss))
	for i, s := range ss {
		q[i] = tomlString(s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// AllPresets returns built-in presets followed by the user's, sorted by name.
// A user preset with a built-in's name replaces it in place.
func (c Config) AllPresets() []encode.Preset {
	var out []encode.Preset
	for _, p := range encode.Builtin {
		if s, ok := c.Presets[p.Name]; ok {
			p.Settings, p.Description = s, "your version of "+p.Name
		}
		out = append(out, p)
	}
	for _, name := range slices.Sorted(maps.Keys(c.Presets)) {
		if !slices.ContainsFunc(encode.Builtin, func(p encode.Preset) bool { return p.Name == name }) {
			out = append(out, encode.Preset{Name: name, Description: "your preset", Settings: c.Presets[name]})
		}
	}
	return out
}

// Preset finds a preset by name.
func (c Config) Preset(name string) (encode.Preset, bool) {
	for _, p := range c.AllPresets() {
		if p.Name == name {
			return p, true
		}
	}
	return encode.Preset{}, false
}

// ExpandHome replaces a leading ~ with the home directory.
func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return p
}

// ShortenHome replaces the home directory prefix with ~.
func ShortenHome(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

// Dirs returns the source directories with ~ expanded.
func (c Config) Dirs() []string {
	out := make([]string, len(c.SourceDirs))
	for i, d := range c.SourceDirs {
		out[i] = ExpandHome(d)
	}
	return out
}

// OutputDirFor returns the folder the output for input goes to.
func (c Config) OutputDirFor(input string) string {
	if c.OutputDir == "" {
		return filepath.Dir(input)
	}
	return ExpandHome(c.OutputDir)
}

// FileName renders the filename template for input.
func (c Config) FileName(input, preset string) string {
	name := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	r := strings.NewReplacer("{name}", name, "{preset}", preset, "{date}", time.Now().Format("2006-01-02"))
	out := strings.ReplaceAll(r.Replace(c.FilenameTemplate), "/", "-")
	if strings.TrimSpace(out) == "" {
		out = name
	}
	return out + ".mp4"
}

// OutputPath is where the output for input goes with the given preset.
func (c Config) OutputPath(input, preset string) string {
	return filepath.Join(c.OutputDirFor(input), c.FileName(input, preset))
}

// Unique returns path, or "name 2.mp4", "name 3.mp4"… if it's taken.
func Unique(path string) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 2; ; i++ {
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			return path
		}
		path = fmt.Sprintf("%s %d%s", base, i, ext)
	}
}
