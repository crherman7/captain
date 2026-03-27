package setup

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/christopherherman/captain/internal/config"
)

// Runner executes setup steps before the deploy pipeline.
type Runner struct {
	stack string
}

// NewRunner creates a setup runner. If stack is non-empty, steps with
// a stacks field are filtered to only run when the stack matches.
func NewRunner(stack string) *Runner {
	return &Runner{stack: stack}
}

// Run executes each setup step in order. For each step:
//   - If check is set and exits 0, the step is skipped (onSkip called).
//   - Otherwise, run is executed (onRun called before, onDone after).
//   - A non-zero exit fails the pipeline.
func (r *Runner) Run(ctx context.Context, steps []config.SetupStep, onSkip, onRun, onDone func(string)) error {
	for _, step := range steps {
		if !r.includeStep(step) {
			continue
		}

		if step.Check != "" {
			cmd := exec.CommandContext(ctx, "sh", "-c", step.Check)
			if err := cmd.Run(); err == nil {
				if onSkip != nil {
					onSkip(step.Name)
				}
				continue
			}
		}

		if onRun != nil {
			onRun(step.Name)
		}

		cmd := exec.CommandContext(ctx, "sh", "-c", step.Run)
		out, err := cmd.CombinedOutput()
		if err != nil {
			if onDone != nil {
				onDone(step.Name)
			}
			return fmt.Errorf("setup step %q failed: %w\n%s", step.Name, err, string(out))
		}

		if onDone != nil {
			onDone(step.Name)
		}
	}
	return nil
}

func (r *Runner) includeStep(step config.SetupStep) bool {
	if r.stack == "" || len(step.Stacks) == 0 {
		return true
	}
	for _, s := range step.Stacks {
		if s == r.stack {
			return true
		}
	}
	return false
}
