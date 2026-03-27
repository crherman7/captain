package build

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type mockRunner struct {
	calls []mockCall
	err   error
}

type mockCall struct {
	Name string
	Args []string
}

func (m *mockRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	m.calls = append(m.calls, mockCall{Name: name, Args: args})
	return nil, m.err
}

func TestBuildxBuilder_BasicBuild(t *testing.T) {
	runner := &mockRunner{}
	b := NewBuildxBuilder(runner)

	err := b.Build(context.Background(), Target{
		Name:     "api",
		Context:  "./services/api",
		ImageTag: "localhost:5001/api:latest",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(runner.calls))
	}

	call := runner.calls[0]
	if call.Name != "docker" {
		t.Errorf("command = %q, want %q", call.Name, "docker")
	}

	args := strings.Join(call.Args, " ")
	if !strings.Contains(args, "buildx build") {
		t.Errorf("expected 'buildx build' in args: %s", args)
	}
	if !strings.Contains(args, "--tag localhost:5001/api:latest") {
		t.Errorf("expected --tag in args: %s", args)
	}
	if !strings.Contains(args, "./services/api") {
		t.Errorf("expected build context in args: %s", args)
	}
}

func TestBuildxBuilder_WithDockerfileAndPlatform(t *testing.T) {
	runner := &mockRunner{}
	b := NewBuildxBuilder(runner)

	err := b.Build(context.Background(), Target{
		Name:       "api",
		Context:    ".",
		Dockerfile: "Dockerfile.prod",
		Platform:   "linux/amd64",
		ImageTag:   "api:v1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args := strings.Join(runner.calls[0].Args, " ")
	if !strings.Contains(args, "--file Dockerfile.prod") {
		t.Errorf("expected --file in args: %s", args)
	}
	if !strings.Contains(args, "--platform linux/amd64") {
		t.Errorf("expected --platform in args: %s", args)
	}
}

func TestBuildxBuilder_Error(t *testing.T) {
	runner := &mockRunner{err: fmt.Errorf("build failed")}
	b := NewBuildxBuilder(runner)

	err := b.Build(context.Background(), Target{
		Name:    "api",
		Context: ".",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

