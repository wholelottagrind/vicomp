// Package tui is the interactive interface: pick a video, tweak the
// explained settings, compress.
package tui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/wholelottagrind/vicomp/internal/config"
	"github.com/wholelottagrind/vicomp/internal/encode"
	"github.com/wholelottagrind/vicomp/internal/job"
	"github.com/wholelottagrind/vicomp/internal/macos"
	"github.com/wholelottagrind/vicomp/internal/media"
)

type screen int

const (
	screenList screen = iota
	screenSettings
	screenEncoding
	screenDone
)

// Settings screen rows: preset, the encode fields, then the two text rows.
const rowPreset = 0

var (
	rowOutDir   = 1 + len(encode.Fields)
	rowFilename = 2 + len(encode.Fields)
	rowCount    = 3 + len(encode.Fields)
)

type editKind int

const (
	editNone editKind = iota
	editOutDir
	editFilename
	editPresetName
)

type probeResult struct {
	info media.Info
	err  error
}

type (
	probeMsg struct {
		path string
		probeResult
	}
	progressMsg encode.Progress
	doneMsg     struct {
		err   error
		notes []string
	}
	clearFlashMsg struct{ id int }
)

// Model is the Bubble Tea model for the whole app.
type Model struct {
	cfg      config.Config
	warnings []string
	width    int
	height   int
	screen   screen

	// list
	files     []media.File
	probes    map[string]probeResult
	isOutput  map[string]bool
	hasOutput map[string]bool
	cursor    int

	// settings
	file       media.File
	presets    []encode.Preset
	presetIdx  int
	s          encode.Settings
	row        int
	outDir     string
	filename   string
	nameEdited bool
	editing    editKind
	input      textinput.Model
	explain    bool
	flash      string
	flashID    int

	// encoding
	outPath  string
	cancel   context.CancelFunc
	events   chan tea.Msg
	prog     encode.Progress
	started  time.Time
	quitting bool
	bar      progress.Model

	// done
	err     error
	elapsed time.Duration
	outSize int64
	notes   []string
}

// New creates the model. If file is non-empty the app opens straight on its
// settings screen.
func New(cfg config.Config, file string) Model {
	m := Model{
		cfg:    cfg,
		probes: map[string]probeResult{},
		input:  textinput.New(),
		bar:    progress.New(progress.WithDefaultBlend(), progress.WithWidth(50)),
	}
	m.input.Prompt = ""
	m.rescan()
	if file != "" {
		f := media.File{Path: file, Name: filepath.Base(file)}
		if fi, err := os.Stat(file); err == nil {
			f.Size, f.ModTime = fi.Size(), fi.ModTime()
		}
		info, err := media.Probe(file)
		m.probes[file] = probeResult{info, err}
		m.openSettings(f)
	}
	return m
}

func (m *Model) rescan() {
	m.files, m.warnings = media.Scan(m.cfg.Dirs(), m.cfg.Extensions)
	m.isOutput, m.hasOutput = job.Classify(m.cfg, m.files, m.cfg.DefaultPreset)
	m.cursor = min(m.cursor, max(0, len(m.files)-1))
}

// probeAll probes every file we haven't probed yet, concurrently.
func (m *Model) probeAll() tea.Cmd {
	var cmds []tea.Cmd
	for _, f := range m.files {
		if _, ok := m.probes[f.Path]; ok {
			continue
		}
		path := f.Path
		cmds = append(cmds, func() tea.Msg {
			info, err := media.Probe(path)
			return probeMsg{path, probeResult{info, err}}
		})
	}
	return tea.Batch(cmds...)
}

func (m Model) Init() tea.Cmd { return m.probeAll() }

func (m *Model) openSettings(f media.File) {
	m.file = f
	m.screen = screenSettings
	m.presets = m.cfg.AllPresets()
	m.presetIdx = max(0, slices.IndexFunc(m.presets, func(p encode.Preset) bool { return p.Name == m.cfg.DefaultPreset }))
	m.s = m.presets[m.presetIdx].Settings
	m.row = rowPreset
	m.outDir = m.cfg.OutputDirFor(f.Path)
	m.nameEdited = false
	m.refreshFilename()
	m.explain, m.flash = false, ""
}

// refreshFilename recomputes the output name from the template unless the
// user typed their own.
func (m *Model) refreshFilename() {
	if m.nameEdited {
		return
	}
	name := m.cfg.FileName(m.file.Path, m.presets[m.presetIdx].Name)
	m.filename = filepath.Base(config.Unique(filepath.Join(m.outDir, name)))
}

func (m *Model) setFlash(s string) tea.Cmd {
	m.flash = s
	m.flashID++
	id := m.flashID
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return clearFlashMsg{id} })
}

func (m Model) info() (media.Info, bool) {
	r, ok := m.probes[m.file.Path]
	return r.info, ok && r.err == nil
}

func (m Model) parts() []encode.Part {
	info, _ := m.info()
	return encode.Build(m.s, info, m.file.Path, filepath.Join(m.outDir, m.filename))
}

func (m Model) modified() bool { return m.s != m.presets[m.presetIdx].Settings }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.bar.SetWidth(min(60, max(20, msg.Width-4)))
		return m, nil
	case probeMsg:
		m.probes[msg.path] = msg.probeResult
		return m, nil
	case clearFlashMsg:
		if msg.id == m.flashID {
			m.flash = ""
		}
		return m, nil
	case progressMsg:
		m.prog = encode.Progress(msg)
		return m, waitFor(m.events)
	case doneMsg:
		return m.finish(msg)
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			if m.screen == screenEncoding {
				m.quitting = true
				m.cancel()
				return m, nil // quit once ffmpeg has stopped and the partial file is gone
			}
			return m, tea.Quit
		}
		switch m.screen {
		case screenList:
			return m.updateList(msg)
		case screenSettings:
			return m.updateSettings(msg)
		case screenEncoding:
			if msg.String() == "esc" {
				m.cancel()
			}
			return m, nil
		case screenDone:
			return m.updateDone(msg)
		}
	}
	if m.editing != editNone {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		return m, tea.Quit
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "down", "j":
		m.cursor = min(len(m.files)-1, m.cursor+1)
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = max(0, len(m.files)-1)
	case "r":
		m.rescan()
		return m, m.probeAll()
	case "enter", "right", "l":
		if len(m.files) > 0 {
			m.openSettings(m.files[m.cursor])
		}
	}
	return m, nil
}

func (m Model) updateSettings(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.editing != editNone {
		return m.updateEditing(msg)
	}
	key := msg.String()
	switch key {
	case "esc", "q":
		m.screen = screenList
		m.rescan()
		return m, m.probeAll()
	case "up", "k":
		m.row = (m.row - 1 + rowCount) % rowCount
	case "down", "j", "tab":
		m.row = (m.row + 1) % rowCount
	case "left", "h", "right", "l":
		dir := 1
		if key == "left" || key == "h" {
			dir = -1
		}
		switch {
		case m.row == rowPreset:
			m.presetIdx = max(0, min(len(m.presets)-1, m.presetIdx+dir))
			m.s = m.presets[m.presetIdx].Settings
			m.refreshFilename()
		case m.row <= len(encode.Fields):
			encode.Fields[m.row-1].Step(&m.s, dir)
		case dir == 1:
			return m.startEdit()
		}
	case "e":
		return m.startEdit()
	case "?":
		m.explain = !m.explain
	case "s":
		m.editing = editPresetName
		m.input.SetValue("")
		m.input.Placeholder = "name"
		return m, m.input.Focus()
	case "d":
		m.cfg.OutputDir = config.ShortenHome(m.outDir)
		if err := config.Save(m.cfg); err != nil {
			return m, m.setFlash("✗ " + err.Error())
		}
		return m, m.setFlash("✓ default output folder is now " + m.cfg.OutputDir)
	case "y":
		if err := macos.CopyText(encode.CommandLine(m.parts(), false)); err != nil {
			return m, m.setFlash("✗ " + err.Error())
		}
		return m, m.setFlash("✓ command copied")
	case "enter":
		return m.startEncode()
	}
	return m, nil
}

func (m Model) startEdit() (tea.Model, tea.Cmd) {
	switch m.row {
	case rowOutDir:
		m.editing = editOutDir
		m.input.SetValue(config.ShortenHome(m.outDir))
	case rowFilename:
		m.editing = editFilename
		m.input.SetValue(m.filename)
	default:
		return m, nil
	}
	m.input.Placeholder = ""
	m.input.CursorEnd()
	return m, m.input.Focus()
}

func (m Model) updateEditing(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.editing = editNone
		m.input.Blur()
		return m, nil
	case "enter":
		val := strings.TrimSpace(m.input.Value())
		kind := m.editing
		m.editing = editNone
		m.input.Blur()
		switch kind {
		case editOutDir:
			if val == "" {
				return m, nil
			}
			dir, err := filepath.Abs(config.ExpandHome(val))
			if err != nil {
				return m, m.setFlash("✗ " + err.Error())
			}
			m.outDir = dir
			m.refreshFilename()
			if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
				return m, m.setFlash("folder doesn't exist yet. It will be created")
			}
		case editFilename:
			if val == "" {
				return m, nil
			}
			val = strings.ReplaceAll(val, "/", "-")
			if !strings.EqualFold(filepath.Ext(val), ".mp4") {
				val += ".mp4"
			}
			m.filename = val
			m.nameEdited = true
		case editPresetName:
			return m.savePreset(val)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) savePreset(name string) (tea.Model, tea.Cmd) {
	if name == "" {
		return m, nil
	}
	if m.cfg.Presets == nil {
		m.cfg.Presets = map[string]encode.Settings{}
	}
	m.cfg.Presets[name] = m.s
	if err := config.Save(m.cfg); err != nil {
		return m, m.setFlash("✗ " + err.Error())
	}
	m.presets = m.cfg.AllPresets()
	m.presetIdx = slices.IndexFunc(m.presets, func(p encode.Preset) bool { return p.Name == name })
	return m, m.setFlash("✓ saved preset " + name)
}

func (m Model) startEncode() (tea.Model, tea.Cmd) {
	r, ok := m.probes[m.file.Path]
	switch {
	case !ok:
		return m, m.setFlash("still reading the video, try again in a moment")
	case r.err != nil:
		return m, m.setFlash("✗ can't read this video: " + r.err.Error())
	}
	m.outPath = filepath.Join(m.outDir, m.filename)
	if m.outPath == m.file.Path {
		return m, m.setFlash("✗ output would overwrite the original. Change the filename")
	}
	if err := os.MkdirAll(m.outDir, 0o755); err != nil {
		return m, m.setFlash("✗ " + err.Error())
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.events = make(chan tea.Msg, 8)
	m.prog = encode.Progress{}
	m.started = time.Now()
	m.screen = screenEncoding

	parts, events, cfg, in, out := m.parts(), m.events, m.cfg, m.file.Path, m.outPath
	go func() {
		err := encode.Run(ctx, parts, func(p encode.Progress) {
			select {
			case events <- progressMsg(p):
			default: // UI is behind; skip this update
			}
		})
		var notes []string
		if err == nil {
			notes = job.After(cfg, in, out)
		}
		events <- doneMsg{err, notes}
	}()
	return m, waitFor(m.events)
}

func waitFor(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func (m Model) finish(msg doneMsg) (tea.Model, tea.Cmd) {
	m.cancel()
	if msg.err != nil {
		os.Remove(m.outPath)
	}
	if m.quitting {
		return m, tea.Quit
	}
	if msg.err == context.Canceled {
		m.screen = screenSettings
		return m, m.setFlash("cancelled")
	}
	m.screen = screenDone
	m.err = msg.err
	m.notes = msg.notes
	m.elapsed = time.Since(m.started)
	if fi, err := os.Stat(m.outPath); err == nil {
		m.outSize = fi.Size()
	}
	return m, nil
}

func (m Model) updateDone(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "enter":
		return m, tea.Quit
	case "r":
		if m.err == nil {
			macos.Reveal(m.outPath)
		}
	case "c":
		if m.err == nil {
			if err := macos.CopyFile(m.outPath); err != nil {
				return m, m.setFlash("✗ " + err.Error())
			}
			return m, m.setFlash("✓ copied to clipboard")
		}
	case "b", "esc":
		if m.err != nil {
			m.screen = screenSettings
			return m, nil
		}
		m.screen = screenList
		m.rescan()
		return m, m.probeAll()
	}
	return m, nil
}
