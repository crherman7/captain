package build

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type mockCall struct {
	Name string
	Args []string
}

func newMockRunner(calls *[]mockCall, err error) *ExecRunner {
	return &ExecRunner{
		runFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			*calls = append(*calls, mockCall{Name: name, Args: args})
			return nil, err
		},
	}
}

func TestBuildxBuilder_BasicBuild(t *testing.T) {
	var calls []mockCall
	b := NewBuildxBuilder(newMockRunner(&calls, nil))

	err := b.Build(context.Background(), Target{
		Name:     "api",
		Context:  "./services/api",
		ImageTag: "localhost:5001/api:latest",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}

	call := calls[0]
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
	var calls []mockCall
	b := NewBuildxBuilder(newMockRunner(&calls, nil))

	err := b.Build(context.Background(), Target{
		Name:       "api",
		Context:    ".",
		Dockerfile: "Dockerfile.prod",
		Platform:   []string{"linux/amd64"},
		ImageTag:   "api:v1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args := strings.Join(calls[0].Args, " ")
	if !strings.Contains(args, "--file Dockerfile.prod") {
		t.Errorf("expected --file in args: %s", args)
	}
	if !strings.Contains(args, "--platform linux/amd64") {
		t.Errorf("expected --platform in args: %s", args)
	}
}

func TestBuildxBuilder_WithCacheRef(t *testing.T) {
	var calls []mockCall
	b := NewBuildxBuilder(newMockRunner(&calls, nil))

	err := b.Build(context.Background(), Target{
		Name:     "api",
		Context:  ".",
		ImageTag: "registry.example.com/api:v1",
		CacheRef: "registry.example.com/api:cache",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args := strings.Join(calls[0].Args, " ")
	if !strings.Contains(args, "--cache-from type=registry,ref=registry.example.com/api:cache") {
		t.Errorf("expected --cache-from in args: %s", args)
	}
	if !strings.Contains(args, "--cache-to type=registry,ref=registry.example.com/api:cache,mode=max") {
		t.Errorf("expected --cache-to in args: %s", args)
	}
}

func TestBuildxBuilder_NoCacheRefByDefault(t *testing.T) {
	var calls []mockCall
	b := NewBuildxBuilder(newMockRunner(&calls, nil))

	err := b.Build(context.Background(), Target{
		Name:     "api",
		Context:  ".",
		ImageTag: "registry.example.com/api:v1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args := strings.Join(calls[0].Args, " ")
	if strings.Contains(args, "--cache-from") || strings.Contains(args, "--cache-to") {
		t.Errorf("expected no cache flags when CacheRef is empty: %s", args)
	}
}

func TestBuildxBuilder_Error(t *testing.T) {
	var calls []mockCall
	b := NewBuildxBuilder(newMockRunner(&calls, fmt.Errorf("build failed")))

	err := b.Build(context.Background(), Target{
		Name:    "api",
		Context: ".",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestShouldPush(t *testing.T) {
	tests := []struct {
		name     string
		imageTag string
		want     bool
	}{
		{
			name:     "local image without registry",
			imageTag: "api:v1",
			want:     false,
		},
		{
			name:     "localhost registry",
			imageTag: "localhost:5001/api:latest",
			want:     true,
		},
		{
			name:     "dotted registry host",
			imageTag: "registry.example.com/myapp:tag",
			want:     true,
		},
		{
			name:     "host port registry without dot",
			imageTag: "registry:5000/myapp:tag",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldPush(tt.imageTag); got != tt.want {
				t.Fatalf("shouldPush(%q) = %t, want %t", tt.imageTag, got, tt.want)
			}
		})
	}
}
