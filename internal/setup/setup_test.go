package setup

import (
	"context"
	"testing"

	"github.com/crherman7/captain/internal/config"
)

func TestRun_CheckPasses_SkipsRun(t *testing.T) {
	r := NewRunner("")
	steps := []config.SetupStep{
		{Name: "already-ready", Check: "true", Run: "false"},
	}

	var skipped []string
	err := r.Run(context.Background(), steps,
		func(name string) { skipped = append(skipped, name) },
		nil, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(skipped) != 1 || skipped[0] != "already-ready" {
		t.Errorf("expected skip of 'already-ready', got %v", skipped)
	}
}

func TestRun_CheckFails_ExecutesRun(t *testing.T) {
	r := NewRunner("")
	steps := []config.SetupStep{
		{Name: "needs-setup", Check: "false", Run: "true"},
	}

	var ran []string
	err := r.Run(context.Background(), steps,
		nil,
		func(name string) { ran = append(ran, name) },
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ran) != 1 || ran[0] != "needs-setup" {
		t.Errorf("expected run of 'needs-setup', got %v", ran)
	}
}

func TestRun_NoCheck_AlwaysRuns(t *testing.T) {
	r := NewRunner("")
	steps := []config.SetupStep{
		{Name: "always-run", Run: "true"},
	}

	var ran []string
	err := r.Run(context.Background(), steps,
		nil,
		func(name string) { ran = append(ran, name) },
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ran) != 1 {
		t.Errorf("expected 1 run, got %d", len(ran))
	}
}

func TestRun_RunFails_ReturnsError(t *testing.T) {
	r := NewRunner("")
	steps := []config.SetupStep{
		{Name: "will-fail", Run: "false"},
	}

	err := r.Run(context.Background(), steps, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error from failing run command")
	}
}

func TestRun_StackFiltering(t *testing.T) {
	r := NewRunner("production")
	steps := []config.SetupStep{
		{Name: "local-only", Run: "false", Stacks: []string{"local"}},
		{Name: "prod-step", Run: "true", Stacks: []string{"production"}},
		{Name: "all-stacks", Run: "true"},
	}

	var ran []string
	err := r.Run(context.Background(), steps,
		nil,
		func(name string) { ran = append(ran, name) },
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ran) != 2 {
		t.Fatalf("expected 2 runs, got %d: %v", len(ran), ran)
	}
	if ran[0] != "prod-step" || ran[1] != "all-stacks" {
		t.Errorf("unexpected run order: %v", ran)
	}
}

func TestRun_EmptySteps(t *testing.T) {
	r := NewRunner("")
	err := r.Run(context.Background(), nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_OnDoneCalled(t *testing.T) {
	r := NewRunner("")
	steps := []config.SetupStep{
		{Name: "step1", Run: "true"},
	}

	var done []string
	err := r.Run(context.Background(), steps,
		nil, nil,
		func(name string) { done = append(done, name) },
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(done) != 1 || done[0] != "step1" {
		t.Errorf("expected done callback for 'step1', got %v", done)
	}
}
