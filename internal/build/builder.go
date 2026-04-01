package build

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Target struct {
	Name       string
	Context    string
	Dockerfile string
	ImageTag   string
	Platform   string
	BuildArgs  map[string]string
	CacheRef   string
}

type Builder interface {
	Build(ctx context.Context, target Target) error
	Bake(ctx context.Context, targets []Target) error
}

type BuildxBuilder struct {
	runner   CommandRunner
	OnOutput func(string)
}

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

func NewBuildxBuilder(runner CommandRunner) *BuildxBuilder {
	return &BuildxBuilder{runner: runner}
}

func (b *BuildxBuilder) Build(ctx context.Context, target Target) error {
	args := []string{"buildx", "build", "--progress=plain"}

	if target.ImageTag != "" {
		args = append(args, "--tag", target.ImageTag)
	}

	if target.Dockerfile != "" {
		args = append(args, "--file", target.Dockerfile)
	}

	if target.Platform != "" {
		args = append(args, "--platform", target.Platform)
	}

	for k, v := range target.BuildArgs {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", k, v))
	}

	if target.CacheRef != "" {
		from, to := cacheRefArgs(target.CacheRef)
		args = append(args, "--cache-from", from)
		args = append(args, "--cache-to", to)
	}

	if shouldPush(target.ImageTag) {
		args = append(args, "--push")
	} else {
		args = append(args, "--load")
	}

	buildContext := target.Context
	if buildContext == "" {
		buildContext = "."
	}
	args = append(args, buildContext)

	if b.OnOutput != nil {
		if r, ok := b.runner.(*ExecRunner); ok {
			r.OnOutput = b.OnOutput
			defer func() { r.OnOutput = nil }()
		}
	}

	_, err := b.runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("building %s: %w", target.Name, err)
	}
	return nil
}

// bakeFile represents the docker buildx bake JSON format.
type bakeFile struct {
	Group  map[string]bakeGroup  `json:"group"`
	Target map[string]bakeTarget `json:"target"`
}

type bakeGroup struct {
	Targets []string `json:"targets"`
}

type bakeTarget struct {
	Context    string            `json:"context"`
	Dockerfile string            `json:"dockerfile,omitempty"`
	Tags       []string          `json:"tags"`
	Platforms  []string          `json:"platforms,omitempty"`
	Args       map[string]string `json:"args,omitempty"`
	Output     []string          `json:"output,omitempty"`
	CacheFrom  []string          `json:"cache-from,omitempty"`
	CacheTo    []string          `json:"cache-to,omitempty"`
}

// Bake builds multiple targets in parallel using docker buildx bake.
func (b *BuildxBuilder) Bake(ctx context.Context, targets []Target) error {
	if len(targets) == 0 {
		return nil
	}

	// Single target — just use regular build
	if len(targets) == 1 {
		return b.Build(ctx, targets[0])
	}

	// Generate bake file
	var targetNames []string
	bf := bakeFile{
		Group:  make(map[string]bakeGroup),
		Target: make(map[string]bakeTarget, len(targets)),
	}

	for _, t := range targets {
		targetNames = append(targetNames, t.Name)
		bt := bakeTarget{
			Context: t.Context,
			Tags:    []string{t.ImageTag},
		}
		if bt.Context == "" {
			bt.Context = "."
		}
		if t.Dockerfile != "" {
			bt.Dockerfile = t.Dockerfile
		}
		if t.Platform != "" {
			bt.Platforms = []string{t.Platform}
		}
		if len(t.BuildArgs) > 0 {
			bt.Args = t.BuildArgs
		}
		if shouldPush(t.ImageTag) {
			bt.Output = []string{"type=registry"}
		} else {
			bt.Output = []string{"type=docker"}
		}
		if t.CacheRef != "" {
			from, to := cacheRefArgs(t.CacheRef)
			bt.CacheFrom = []string{from}
			bt.CacheTo = []string{to}
		}
		bf.Target[t.Name] = bt
	}

	bf.Group["default"] = bakeGroup{Targets: targetNames}

	// Write bake file to temp dir
	data, err := json.MarshalIndent(bf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling bake file: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "captain-bake-*")
	if err != nil {
		return fmt.Errorf("creating temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	bakePath := filepath.Join(tmpDir, "docker-bake.json")
	if err := os.WriteFile(bakePath, data, 0644); err != nil {
		return fmt.Errorf("writing bake file: %w", err)
	}

	// Run docker buildx bake
	args := []string{"buildx", "bake", "--progress=plain", "-f", bakePath}

	if b.OnOutput != nil {
		if r, ok := b.runner.(*ExecRunner); ok {
			r.OnOutput = b.OnOutput
			defer func() { r.OnOutput = nil }()
		}
	}

	_, err = b.runner.Run(ctx, "docker", args...)
	if err != nil {
		return fmt.Errorf("bake: %w", err)
	}
	return nil
}

// cacheRefArgs returns the --cache-from and --cache-to values for a registry cache ref.
func cacheRefArgs(ref string) (from, to string) {
	return "type=registry,ref=" + ref, "type=registry,ref=" + ref + ",mode=max"
}

// shouldPush returns true if the image tag references a remote registry.
// It checks whether the first path segment looks like a registry hostname
// (contains a dot or is "localhost"), indicating the image should be pushed.
func shouldPush(imageTag string) bool {
	if imageTag == "" {
		return false
	}
	parts := strings.SplitN(imageTag, "/", 2)
	if len(parts) < 2 {
		return false // no slash means local image like "myapp:v1"
	}
	host := parts[0]
	return strings.Contains(host, ".") || strings.Contains(host, ":") || host == "localhost"
}
