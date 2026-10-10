// vicomp compresses screen recordings with ffmpeg, explaining every setting.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/wholelottagrind/vicomp/internal/config"
	"github.com/wholelottagrind/vicomp/internal/encode"
	"github.com/wholelottagrind/vicomp/internal/job"
	"github.com/wholelottagrind/vicomp/internal/macos"
	"github.com/wholelottagrind/vicomp/internal/media"
	"github.com/wholelottagrind/vicomp/internal/tui"
)

const usage = `vicomp: compress screen recordings for sending, with every setting explained.

Usage:
  vicomp                 pick a recording from your folders and compress it
  vicomp <file>          open the settings for a specific video
  vicomp last [flags]    compress the newest recording with the default preset, no questions
      -preset NAME         use another preset
      -dry-run             only print the ffmpeg command and what each part does
      -force               compress again even if it was already done
  vicomp presets         list presets
  vicomp config          open the config file in your editor
  vicomp config path     print where the config file is
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "vicomp:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		fmt.Print(usage)
		return nil
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return errors.New("ffmpeg not found. Install it with: brew install ffmpeg")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return runTUI(cfg, "")
	}
	switch args[0] {
	case "last":
		return runLast(cfg, args[1:])
	case "presets":
		for _, p := range cfg.AllPresets() {
			def := ""
			if p.Name == cfg.DefaultPreset {
				def = " (default)"
			}
			s := p.Settings
			fmt.Printf("%-10s %s%s\n           %s %s, quality %d, %s, max %s, %s, audio %s\n",
				p.Name, p.Description, def, s.Codec, s.Encoder, s.Quality, s.Speed,
				orOriginal(s.MaxRes, "p"), orOriginal(s.MaxFPS, " fps"), s.Audio)
		}
		return nil
	case "config":
		if len(args) > 1 && args[1] == "path" {
			fmt.Println(config.Path())
			return nil
		}
		return openEditor(config.Path())
	}

	path := args[0]
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("unknown command or file %q (see vicomp --help)", path)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	return runTUI(cfg, abs)
}

func runTUI(cfg config.Config, file string) error {
	_, err := tea.NewProgram(tui.New(cfg, file)).Run()
	return err
}

func orOriginal(v int, unit string) string {
	if v == 0 {
		return "original"
	}
	return fmt.Sprint(v, unit)
}

func openEditor(path string) error {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	var cmd *exec.Cmd
	if editor != "" {
		cmd = exec.Command("sh", "-c", editor+` "$1"`, "sh", path)
	} else {
		cmd = exec.Command("open", "-t", path)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func runLast(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("last", flag.ContinueOnError)
	presetName := fs.String("preset", cfg.DefaultPreset, "preset to use")
	dryRun := fs.Bool("dry-run", false, "only print the command")
	force := fs.Bool("force", false, "compress again even if already done")
	if err := fs.Parse(args); err != nil {
		return err
	}
	preset, ok := cfg.Preset(*presetName)
	if !ok {
		return fmt.Errorf("no preset called %q (see vicomp presets)", *presetName)
	}

	files, warnings := media.Scan(cfg.Dirs(), cfg.Extensions)
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	isOutput, _ := job.Classify(cfg, files, preset.Name)
	var src *media.File
	for i := range files {
		if !isOutput[files[i].Path] {
			src = &files[i]
			break
		}
	}
	if src == nil {
		return fmt.Errorf("no videos found in %s", strings.Join(cfg.SourceDirs, ", "))
	}

	out := cfg.OutputPath(src.Path, preset.Name)
	if _, err := os.Stat(out); err == nil && !*force && !*dryRun {
		fmt.Printf("%s was already compressed:\n  %s\n", src.Name, config.ShortenHome(out))
		if cfg.CopyToClipboard {
			if err := macos.CopyFile(out); err != nil {
				return err
			}
			fmt.Println("📋 copied to clipboard again. Use -force to re-compress")
		}
		return nil
	}

	info, err := media.Probe(src.Path)
	if err != nil {
		return fmt.Errorf("%s: %w", src.Name, err)
	}
	parts := encode.Build(preset.Settings, info, src.Path, out)

	fmt.Printf("%s  (%s · %s)\npreset %s: %s\n", src.Name, media.HumanSize(src.Size), info.Summary(), preset.Name, preset.Description)
	if *dryRun {
		fmt.Println("\n" + encode.CommandLine(parts, false) + "\n")
		for i, p := range parts {
			args := strings.Join(p.Args, " ")
			if i == 0 || i == len(parts)-1 {
				args = strings.Replace(args, filepath.Dir(p.Args[len(p.Args)-1])+"/", "", 1)
			}
			fmt.Printf("  %-28s %s\n", args, p.Why)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	start := time.Now()
	err = encode.Run(ctx, parts, func(p encode.Progress) {
		pct := 0.0
		if info.Duration > 0 {
			pct = min(100, 100*float64(p.Done)/float64(info.Duration))
		}
		fmt.Printf("\r  %3.0f%%  %s / %s  %.1f×  %s   ", pct, media.HumanDuration(p.Done),
			media.HumanDuration(info.Duration), p.Speed, media.HumanSize(p.Size))
	})
	fmt.Println()
	if err != nil {
		os.Remove(out)
		if ctx.Err() != nil {
			return errors.New("cancelled")
		}
		return err
	}

	fi, err := os.Stat(out)
	if err != nil {
		return err
	}
	fmt.Printf("✓ %s → %s in %s\n  %s\n", media.HumanSize(src.Size), media.HumanSize(fi.Size()),
		time.Since(start).Round(time.Second), config.ShortenHome(out))
	for _, n := range job.After(cfg, src.Path, out) {
		fmt.Println(n)
	}
	return nil
}
