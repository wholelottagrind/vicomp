package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/wholelottagrind/vicomp/internal/config"
	"github.com/wholelottagrind/vicomp/internal/encode"
	"github.com/wholelottagrind/vicomp/internal/media"
)

var (
	accent    = lipgloss.Color("212")
	titleSt   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	dimSt     = lipgloss.NewStyle().Faint(true)
	selSt     = lipgloss.NewStyle().Foreground(accent).Bold(true)
	valueSt   = lipgloss.NewStyle().Bold(true)
	okSt      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnSt    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errSt     = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	cmdSt     = lipgloss.NewStyle().Foreground(lipgloss.Color("110"))
	keySt     = lipgloss.NewStyle().Foreground(accent)
	aboutSt   = lipgloss.NewStyle().BorderStyle(lipgloss.ThickBorder()).BorderLeft(true).BorderForeground(accent).PaddingLeft(1)
	labelSize = 15
	valueSize = 20
)

func (m Model) View() tea.View {
	var s string
	switch m.screen {
	case screenList:
		s = m.listView()
	case screenSettings:
		s = m.settingsView()
	case screenEncoding:
		s = m.encodingView()
	case screenDone:
		s = m.doneView()
	}
	v := tea.NewView(s)
	v.AltScreen = true
	v.WindowTitle = "vicomp"
	if m.screen == screenEncoding && m.prog.Done > 0 {
		if info, ok := m.info(); ok && info.Duration > 0 {
			v.ProgressBar = tea.NewProgressBar(tea.ProgressBarDefault, int(100*m.fraction(info)))
		}
	}
	return v
}

func (m Model) w() int {
	if m.width == 0 {
		return 100
	}
	return m.width
}

func keys(pairs ...string) string { return keysWrapped(0, pairs...) }

// keysWrapped renders "key action" pairs, breaking lines only between pairs
// when width > 0.
func keysWrapped(width int, pairs ...string) string {
	sep := dimSt.Render(" · ")
	var lines []string
	line := ""
	for i := 0; i+1 < len(pairs); i += 2 {
		item := keySt.Render(pairs[i]) + " " + dimSt.Render(pairs[i+1])
		switch {
		case line == "":
			line = item
		case width > 0 && lipgloss.Width(line+sep+item) > width:
			lines = append(lines, line)
			line = item
		default:
			line += sep + item
		}
	}
	return strings.Join(append(lines, line), "\n")
}

// wrapWords wraps plain text at spaces only (never inside a word like -pix_fmt).
func wrapWords(s string, width int) string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line != "" && lipgloss.Width(line)+1+lipgloss.Width(word) > width {
			lines = append(lines, line)
			line = word
			continue
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	return strings.Join(append(lines, line), "\n")
}

func wrap(s string, width int) string {
	return lipgloss.NewStyle().Width(width).Render(s)
}

// ---- list ----

func (m Model) listView() string {
	var b strings.Builder
	dirs := make([]string, len(m.cfg.SourceDirs))
	copy(dirs, m.cfg.SourceDirs)
	b.WriteString(titleSt.Render("vicomp") + dimSt.Render("  pick a video  ·  "+strings.Join(dirs, ", ")) + "\n\n")

	for _, w := range m.warnings {
		b.WriteString(warnSt.Render("⚠ "+w) + "\n")
	}
	if len(m.files) == 0 {
		b.WriteString("No videos found. Record one with ⇧⌘5, or add folders to source_dirs in\n" +
			config.ShortenHome(config.Path()) + " (run: vicomp config)\n\n")
		b.WriteString(keys("r", "rescan", "q", "quit"))
		return b.String()
	}

	// Columns: marker, name, size, duration, resolution, fps, age.
	const fixed = 2 + 9 + 9 + 11 + 8 + 10
	nameW := max(20, m.w()-fixed-5)
	visible := max(3, m.height-7)
	start := max(0, min(m.cursor-visible/2, len(m.files)-visible))
	end := min(len(m.files), start+visible)

	for i := start; i < end; i++ {
		f := m.files[i]
		name := ansi.Truncate(f.Name, nameW-2, "…")
		var details string
		switch r, ok := m.probes[f.Path]; {
		case !ok:
			details = dimSt.Render(fmt.Sprintf("%9s", "…"))
		case r.err != nil:
			details = errSt.Render("  can't read")
		default:
			details = fmt.Sprintf("%9s%11s%8s", media.HumanDuration(r.info.Duration),
				fmt.Sprintf("%d×%d", r.info.Width, r.info.Height), fmt.Sprintf("%.0ffps", r.info.FPS))
		}
		line := fmt.Sprintf("%-*s%9s%s%10s", nameW, name, media.HumanSize(f.Size), details, media.HumanAge(f.ModTime))

		marker, status := "  ", ""
		switch {
		case m.isOutput[f.Path]:
			status = dimSt.Render("  output")
		case m.hasOutput[f.Path]:
			status = okSt.Render("  ✓")
		}
		switch {
		case i == m.cursor:
			marker = selSt.Render("▸ ")
			line = selSt.Render(line)
		case m.isOutput[f.Path]:
			line = dimSt.Render(line)
		}
		b.WriteString(marker + line + status + "\n")
	}
	if len(m.files) > visible {
		b.WriteString(dimSt.Render(fmt.Sprintf("  %d of %d", m.cursor+1, len(m.files))) + "\n")
	}
	if len(m.hasOutput) > 0 {
		b.WriteString("\n" + okSt.Render("✓") + dimSt.Render(" already compressed") + "\n")
	}
	b.WriteString("\n")
	b.WriteString(keys("↑↓", "select", "enter", "open", "r", "rescan", "q", "quit"))
	return b.String()
}

// ---- settings ----

func (m Model) settingsView() string {
	w := m.w()
	var b strings.Builder
	b.WriteString(titleSt.Render("vicomp") + dimSt.Render("  ›  ") + valueSt.Render(ansi.Truncate(m.file.Name, w-12, "…")) + "\n")
	info, _ := m.info()
	switch r, probed := m.probes[m.file.Path]; {
	case !probed:
		b.WriteString(dimSt.Render("reading video…") + "\n\n")
	case r.err != nil:
		b.WriteString(errSt.Render("can't read video: "+r.err.Error()) + "\n\n")
	default:
		b.WriteString(dimSt.Render(media.HumanSize(m.file.Size)+" · "+info.Summary()) + "\n\n")
	}

	preset := m.presets[m.presetIdx]
	presetVal, presetNote := preset.Name, preset.Description
	if m.modified() {
		presetVal += "*"
		presetNote = "changed from " + preset.Name + ". Press s to save as your own preset"
	}
	b.WriteString(m.optionRow(rowPreset, "Preset", presetVal, presetNote, w))
	for i, f := range encode.Fields {
		b.WriteString(m.optionRow(i+1, f.Name, f.Value(m.s), f.Note(m.s, info), w))
	}
	b.WriteString(m.textRow(rowOutDir, "Output folder", config.ShortenHome(m.outDir), editOutDir, w))
	b.WriteString(m.textRow(rowFilename, "Filename", m.filename, editFilename, w))
	b.WriteString("\n")

	if m.explain {
		b.WriteString(m.explainView(w))
	} else {
		b.WriteString(aboutSt.Render(wrap(m.about(), min(w-4, 90))) + "\n\n")
		cmd := encode.CommandLine(m.parts(), true)
		b.WriteString(cmdSt.Render(wrapWords("$ "+cmd, min(w, 120))) + "\n")
	}
	b.WriteString("\n")

	switch {
	case m.editing == editPresetName:
		b.WriteString("Save current settings as preset: " + m.input.View() + "  " + keys("enter", "save", "esc", "cancel") + "\n")
	case m.editing != editNone:
		b.WriteString(keys("enter", "done", "esc", "cancel") + "\n")
	case m.flash != "":
		b.WriteString(m.flash + "\n")
	default:
		b.WriteString("\n")
	}
	if m.editing == editNone {
		b.WriteString(keysWrapped(w, "↑↓", "move", "←→", "change", "e", "edit path", "enter", "compress",
			"?", "explain command", "s", "save preset", "d", "make folder default", "y", "copy command", "esc", "back"))
	}
	return b.String()
}

func (m Model) optionRow(row int, label, value, note string, w int) string {
	sel := row == m.row
	val := "◀ " + value + " ▶"
	label = fmt.Sprintf("%-*s", labelSize, label)
	val = fmt.Sprintf("%-*s", valueSize, val)
	noteW := max(0, w-labelSize-valueSize-4)
	note = ansi.Truncate(note, noteW, "…")
	if sel {
		return selSt.Render("▸ "+label) + selSt.Render(val) + "  " + note + "\n"
	}
	return "  " + label + valueSt.Render(val) + "  " + dimSt.Render(note) + "\n"
}

func (m Model) textRow(row int, label, value string, kind editKind, w int) string {
	sel := row == m.row
	label = fmt.Sprintf("%-*s", labelSize, label)
	if m.editing == kind {
		m.input.SetWidth(max(10, w-labelSize-6))
		return selSt.Render("▸ "+label) + m.input.View() + "\n"
	}
	value = ansi.Truncate(value, max(10, w-labelSize-4), "…")
	note := ""
	if row == rowFilename {
		if _, err := statFile(m.outDir, m.filename); err == nil {
			note = warnSt.Render("  ⚠ exists, will be replaced")
		}
	}
	if sel {
		return selSt.Render("▸ "+label) + selSt.Render(value) + note + "\n"
	}
	return "  " + label + value + note + "\n"
}

// about is the longer explanation for the selected row.
func (m Model) about() string {
	switch {
	case m.row == rowPreset:
		return "Preset: a ready-made combination of the settings below. Pick one, then tweak " +
			"anything you like. Built-ins: telegram (balanced), tiny, quality, fast (hardware), classic " +
			"(your old command). Press s to save your own; the default lives in the config file."
	case m.row <= len(encode.Fields):
		f := encode.Fields[m.row-1]
		return f.Name + ": " + f.About
	case m.row == rowOutDir:
		return "Output folder: where the compressed video is saved. Press e to change it, " +
			"and d to make it the default for next time (empty in the config = next to the original)."
	default:
		return "Filename: the name of the compressed video. Press e to change it. The default comes " +
			"from filename_template in the config. If a file with this name exists, a number is added."
	}
}

func (m Model) explainView(w int) string {
	var b strings.Builder
	b.WriteString(valueSt.Render("What each part of the command does:") + "\n")
	const flagW = 26
	parts := m.parts()
	indent := strings.Repeat(" ", flagW+2)
	for i, p := range parts {
		var words []string
		for j, a := range p.Args {
			if (i == 0 && j == 1) || i == len(parts)-1 {
				a = filepath.Base(a) // full paths are shown in the rows above
			}
			words = append(words, encode.ShellQuote(a))
		}
		flag := ansi.Truncate(strings.Join(words, " "), flagW-1, "…")
		why := strings.ReplaceAll(wrapWords(p.Why, max(20, min(w, 120)-flagW-2)), "\n", "\n"+indent)
		b.WriteString("  " + cmdSt.Render(fmt.Sprintf("%-*s", flagW, flag)) + dimSt.Render(why) + "\n")
	}
	return b.String()
}

// ---- encoding ----

func (m Model) fraction(info media.Info) float64 {
	if info.Duration <= 0 {
		return 0
	}
	return min(1, float64(m.prog.Done)/float64(info.Duration))
}

func (m Model) encodingView() string {
	info, _ := m.info()
	frac := m.fraction(info)
	var b strings.Builder
	b.WriteString(titleSt.Render("vicomp") + dimSt.Render("  compressing  ") + valueSt.Render(m.file.Name) + "\n")
	b.WriteString(dimSt.Render("→ "+config.ShortenHome(m.outPath)) + "\n\n")
	b.WriteString(m.bar.ViewAs(frac) + "\n\n")

	elapsed := time.Since(m.started)
	stats := []string{media.HumanDuration(m.prog.Done) + " / " + media.HumanDuration(info.Duration)}
	if m.prog.Speed > 0 {
		stats = append(stats, fmt.Sprintf("%.1f× real time", m.prog.Speed))
	}
	if m.prog.Size > 0 {
		stats = append(stats, media.HumanSize(m.prog.Size)+" so far")
	}
	if frac > 0.02 {
		left := time.Duration(float64(elapsed) * (1 - frac) / frac)
		stats = append(stats, "~"+media.HumanDuration(left)+" left")
	}
	b.WriteString(strings.Join(stats, dimSt.Render(" · ")) + "\n\n")
	if m.quitting {
		b.WriteString(dimSt.Render("stopping…"))
	} else {
		b.WriteString(keys("esc", "cancel"))
	}
	return b.String()
}

// ---- done ----

func (m Model) doneView() string {
	var b strings.Builder
	if m.err != nil {
		b.WriteString(errSt.Render("✗ Compression failed") + "\n\n")
		b.WriteString(wrap(m.err.Error(), m.w()) + "\n\n")
		b.WriteString(keys("b", "back to settings", "q", "quit"))
		return b.String()
	}
	b.WriteString(okSt.Render("✓ Done") + dimSt.Render(" in "+m.elapsed.Round(time.Second).String()) + "\n\n")
	saved := 100 * (1 - float64(m.outSize)/float64(max(1, m.file.Size)))
	b.WriteString(fmt.Sprintf("%s → %s  ", media.HumanSize(m.file.Size), valueSt.Render(media.HumanSize(m.outSize))))
	if saved >= 0 {
		b.WriteString(okSt.Render(fmt.Sprintf("(−%.0f%%)", saved)) + "\n")
	} else {
		b.WriteString(warnSt.Render(fmt.Sprintf("(+%.0f%%: bigger than the original! Try a higher quality number or a lower resolution)", -saved)) + "\n")
	}
	b.WriteString(config.ShortenHome(m.outPath) + "\n\n")
	for _, n := range m.notes {
		b.WriteString(n + "\n")
	}
	if len(m.notes) > 0 {
		b.WriteString("\n")
	}
	if m.flash != "" {
		b.WriteString(m.flash + "\n")
	}
	b.WriteString(keys("enter", "quit", "c", "copy file", "r", "reveal in Finder", "b", "compress another"))
	return b.String()
}

func statFile(dir, name string) (os.FileInfo, error) {
	return os.Stat(filepath.Join(dir, name))
}
