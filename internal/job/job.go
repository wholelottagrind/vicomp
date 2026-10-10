// Package job holds the pieces shared by the TUI and "vicomp last":
// recognising vicomp's own outputs and the after-encode actions.
package job

import (
	"os"

	"github.com/wholelottagrind/vicomp/internal/config"
	"github.com/wholelottagrind/vicomp/internal/macos"
	"github.com/wholelottagrind/vicomp/internal/media"
)

// Classify works out, using the filename template with the given preset,
// which files are vicomp outputs and which originals already have one.
func Classify(cfg config.Config, files []media.File, preset string) (isOutput, hasOutput map[string]bool) {
	isOutput, hasOutput = map[string]bool{}, map[string]bool{}
	present := map[string]bool{}
	for _, f := range files {
		present[f.Path] = true
	}
	for _, f := range files {
		out := cfg.OutputPath(f.Path, preset)
		if out == f.Path {
			continue
		}
		if present[out] {
			isOutput[out] = true
			hasOutput[f.Path] = true
		} else if fileExists(out) {
			hasOutput[f.Path] = true // output lives outside the source folders
		}
	}
	return isOutput, hasOutput
}

// After runs the configured post-encode actions and describes what happened.
func After(cfg config.Config, input, output string) (notes []string) {
	if cfg.CopyToClipboard {
		if err := macos.CopyFile(output); err != nil {
			notes = append(notes, "✗ couldn't copy to clipboard: "+err.Error())
		} else {
			notes = append(notes, "📋 copied to clipboard. Paste it into Telegram with ⌘V")
		}
	}
	if cfg.RevealInFinder {
		if err := macos.Reveal(output); err != nil {
			notes = append(notes, "✗ couldn't reveal in Finder: "+err.Error())
		}
	}
	if cfg.TrashOriginal {
		if err := macos.Trash(input); err != nil {
			notes = append(notes, "✗ couldn't move the original to Trash: "+err.Error())
		} else {
			notes = append(notes, "🗑  original moved to Trash")
		}
	}
	return notes
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
