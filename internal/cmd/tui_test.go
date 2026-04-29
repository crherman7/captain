package cmd

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIBuffersMessagesUntilWindowSize(t *testing.T) {
	model := newTuiModel()

	next, _ := model.Update(msgHeader{text: "Deploy"})
	model = next.(tuiModel)

	next, _ = model.Update(msgServiceStart{name: "deploy/postgres", message: "deploying..."})
	model = next.(tuiModel)

	if len(model.lines) != 0 {
		t.Fatalf("expected no rendered lines before window size, got %d", len(model.lines))
	}
	if len(model.pending) != 2 {
		t.Fatalf("expected 2 buffered messages, got %d", len(model.pending))
	}

	next, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = next.(tuiModel)

	if !model.ready {
		t.Fatal("expected model to become ready after window size")
	}
	if len(model.pending) != 0 {
		t.Fatalf("expected buffered messages to flush, got %d pending", len(model.pending))
	}
	if len(model.lines) != 2 {
		t.Fatalf("expected 2 rendered lines after flush, got %d", len(model.lines))
	}
}

func TestTUIUpdatesServiceInPlace(t *testing.T) {
	model := newTuiModel()

	next, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model = next.(tuiModel)

	next, _ = model.Update(msgServiceStart{name: "deploy/postgres", message: "deploying..."})
	model = next.(tuiModel)

	next, _ = model.Update(msgServiceUpdate{name: "deploy/postgres", message: "deploying observedGeneration..."})
	model = next.(tuiModel)

	next, _ = model.Update(msgServiceDone{name: "deploy/postgres", icon: "✔", message: "deployed"})
	model = next.(tuiModel)

	if len(model.lines) != 1 {
		t.Fatalf("expected 1 service line, got %d", len(model.lines))
	}

	line := model.lines[0]
	if line.status != lineDone {
		t.Fatalf("expected line status %v, got %v", lineDone, line.status)
	}
	if line.message != "deployed" {
		t.Fatalf("expected final message %q, got %q", "deployed", line.message)
	}
}
