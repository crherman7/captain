package cmd

import (
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/crherman7/captain/internal/build"
)

var (
	spinnerFrames = []string{"◐", "◓", "◑", "◒"}

	iconStyle    = lipgloss.NewStyle().Width(2)
	nameStyle    = lipgloss.NewStyle().Width(22)
	messageStyle = lipgloss.NewStyle().Width(40)
	dimStyle     = lipgloss.NewStyle().Faint(true)
	greenStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	redStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	cyanStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	boldStyle    = lipgloss.NewStyle().Bold(true)
)

// --- Bubbletea messages ---

type msgServiceStart struct{ name, message string }
type msgServiceUpdate struct{ name, message string }
type msgServiceDone struct{ name, icon, message string }
type msgServiceSkip struct{ name, message string }
type msgHeader struct{ text string }
type msgError struct{ err error }
type msgTick struct{}
type msgQuit struct{}

// --- Line types ---

type lineKind int

const (
	lineKindHeader lineKind = iota
	lineKindService
)

type lineStatus int

const (
	lineSpinning lineStatus = iota
	lineDone
	lineSkipped
)

type tuiLine struct {
	kind    lineKind
	text    string // for headers
	name    string
	status  lineStatus
	message string
	icon    string
	started time.Time
	elapsed time.Duration
}

// --- Bubbletea model ---

type tuiModel struct {
	lines         []tuiLine
	index         map[string]int // service name -> line index
	pending       []tea.Msg
	ready         bool
	frame         int
	errMsg        string
	buildFailures []build.BuildFailure
}

func newTuiModel() tuiModel {
	return tuiModel{
		index: make(map[string]int),
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tickCmd()
}

func tickCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(_ time.Time) tea.Msg {
		return msgTick{}
	})
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case msgTick:
		m.frame++
		return m, tickCmd()

	case tea.WindowSizeMsg:
		tuiDebugf("window-size width=%d height=%d ready=%t pending=%d lines=%d", msg.Width, msg.Height, m.ready, len(m.pending), len(m.lines))
		if !m.ready {
			m.ready = true
			for _, pending := range m.pending {
				m = m.applyMessage(pending)
			}
			m.pending = nil
		}
		return m, nil

	case msgHeader, msgServiceStart, msgServiceUpdate, msgServiceDone, msgServiceSkip, msgError:
		if !m.ready {
			tuiDebugf("buffer msg=%T pending=%d lines=%d", msg, len(m.pending)+1, len(m.lines))
			m.pending = append(m.pending, msg)
			return m, nil
		}
		m = m.applyMessage(msg)
		return m, nil

	case msgQuit:
		return m, tea.Quit

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m tuiModel) applyMessage(msg tea.Msg) tuiModel {
	switch msg := msg.(type) {
	case msgHeader:
		tuiDebugf("apply header text=%q lines_before=%d", msg.text, len(m.lines))
		m.lines = append(m.lines, tuiLine{
			kind: lineKindHeader,
			text: msg.text,
		})

	case msgServiceStart:
		tuiDebugf("apply start key=%q display=%q exists=%t lines_before=%d", msg.name, displayName(msg.name), hasKey(m.index, msg.name), len(m.lines))
		if idx, ok := m.index[msg.name]; ok {
			m.lines[idx].status = lineSpinning
			m.lines[idx].message = msg.message
			m.lines[idx].started = time.Now()
		} else {
			m.index[msg.name] = len(m.lines)
			m.lines = append(m.lines, tuiLine{
				kind:    lineKindService,
				name:    displayName(msg.name),
				status:  lineSpinning,
				message: msg.message,
				started: time.Now(),
			})
		}

	case msgServiceUpdate:
		tuiDebugf("apply update key=%q display=%q exists=%t", msg.name, displayName(msg.name), hasKey(m.index, msg.name))
		if idx, ok := m.index[msg.name]; ok {
			m.lines[idx].message = msg.message
		}

	case msgServiceDone:
		tuiDebugf("apply done key=%q display=%q exists=%t", msg.name, displayName(msg.name), hasKey(m.index, msg.name))
		if idx, ok := m.index[msg.name]; ok {
			m.lines[idx].status = lineDone
			m.lines[idx].icon = msg.icon
			m.lines[idx].message = msg.message
			m.lines[idx].elapsed = time.Since(m.lines[idx].started)
		}

	case msgServiceSkip:
		tuiDebugf("apply skip key=%q display=%q exists=%t lines_before=%d", msg.name, displayName(msg.name), hasKey(m.index, msg.name), len(m.lines))
		if idx, ok := m.index[msg.name]; ok {
			m.lines[idx].status = lineSkipped
			m.lines[idx].message = msg.message
			m.lines[idx].icon = "-"
		} else {
			m.index[msg.name] = len(m.lines)
			m.lines = append(m.lines, tuiLine{
				kind:    lineKindService,
				name:    displayName(msg.name),
				status:  lineSkipped,
				message: msg.message,
				icon:    "-",
			})
		}

	case msgError:
		tuiDebugf("apply error err=%q", msg.err.Error())
		var be *build.BuildError
		if errors.As(msg.err, &be) && len(be.Failures) > 0 {
			m.buildFailures = be.Failures
		} else {
			m.errMsg = msg.err.Error()
		}
	}

	return m
}

func hasKey(index map[string]int, key string) bool {
	_, ok := index[key]
	return ok
}

func (m tuiModel) View() string {
	var b strings.Builder

	for _, line := range m.lines {
		switch line.kind {
		case lineKindHeader:
			b.WriteString("\n")
			b.WriteString(boldStyle.Render(line.text))
			b.WriteString("\n\n")

		case lineKindService:
			b.WriteString("  ")
			switch line.status {
			case lineSpinning:
				frame := spinnerFrames[m.frame%len(spinnerFrames)]
				b.WriteString(cyanStyle.Render(iconStyle.Render(frame)))
				b.WriteString(nameStyle.Render(line.name))
				msg := line.message
				if len(msg) > 40 {
					msg = msg[:37] + "..."
				}
				b.WriteString(messageStyle.Render(msg))

			case lineDone:
				var styled string
				if line.icon == "✗" {
					styled = redStyle.Render(iconStyle.Render(line.icon))
				} else {
					styled = greenStyle.Render(iconStyle.Render(line.icon))
				}
				b.WriteString(styled)
				b.WriteString(nameStyle.Render(line.name))
				b.WriteString(messageStyle.Render(line.message))
				b.WriteString(dimStyle.Render(formatDuration(line.elapsed)))

			case lineSkipped:
				b.WriteString(dimStyle.Render(iconStyle.Render("-")))
				b.WriteString(nameStyle.Render(line.name))
				b.WriteString(dimStyle.Render(line.message))
			}
			b.WriteString("\n")
		}
	}

	if len(m.buildFailures) > 0 {
		b.WriteString("\n")
		for _, f := range m.buildFailures {
			label := f.Target
			if label == "" {
				label = "build"
			}
			b.WriteString(redStyle.Render("  ✗ " + label))
			if f.DockerfileAt != "" {
				b.WriteString(dimStyle.Render("  " + f.DockerfileAt))
			}
			b.WriteString("\n")
			if f.Step != "" {
				b.WriteString(dimStyle.Render("    step: " + f.Step))
				b.WriteString("\n")
			}
			if f.Snippet != "" {
				for _, line := range strings.Split(f.Snippet, "\n") {
					b.WriteString("    ")
					b.WriteString(line)
					b.WriteString("\n")
				}
			}
			if f.Error != "" {
				b.WriteString(redStyle.Render("    " + f.Error))
				b.WriteString("\n")
			}
		}
	}

	if m.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(redStyle.Render("  ✗ Error: "))
		b.WriteString(m.errMsg)
		b.WriteString("\n")
	}

	return b.String()
}

// --- TUI wrapper implementing UI interface ---

type tui struct {
	program  *tea.Program
	done     chan struct{}
	mu       sync.RWMutex
	final    string
	warnings *tuiWarningCollector
	stdlog   *tuiStdLogCapture
}

// NewTUI creates a bubbletea-powered interactive UI.
func NewTUI() UI {
	model := newTuiModel()
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithOutput(os.Stdout),
	)

	t := &tui{
		program:  p,
		done:     make(chan struct{}),
		warnings: installTUIWarningHandler(),
		stdlog:   installTUIStdLogCapture(),
	}

	go func() {
		finalModel, _ := p.Run()
		if model, ok := finalModel.(tuiModel); ok {
			t.mu.Lock()
			t.final = model.View()
			t.mu.Unlock()
		}
		close(t.done)
	}()

	return t
}

func (t *tui) Header(msg string) {
	t.program.Send(msgHeader{text: msg})
}

func (t *tui) ServiceStart(name, message string) {
	t.program.Send(msgServiceStart{name: name, message: message})
}

func (t *tui) ServiceUpdate(name, message string) {
	t.program.Send(msgServiceUpdate{name: name, message: message})
}

func (t *tui) ServiceDone(name, icon, message string) {
	t.program.Send(msgServiceDone{name: name, icon: icon, message: message})
}

func (t *tui) ServiceSkip(name, message string) {
	t.program.Send(msgServiceSkip{name: name, message: message})
}

func (t *tui) Error(err error) {
	t.program.Send(msgError{err: err})
}

func (t *tui) Flush() {
	t.program.Send(msgQuit{})
	<-t.done
	restoreDefaultWarningHandler()
	if t.stdlog != nil {
		t.stdlog.restore()
	}

	t.mu.RLock()
	final := t.final
	t.mu.RUnlock()

	if final != "" {
		_, _ = os.Stdout.WriteString(final)
	}
	if t.warnings != nil {
		t.warnings.FlushTo(os.Stdout)
	}
	if t.stdlog != nil {
		t.stdlog.FlushTo(os.Stdout)
	}
}
