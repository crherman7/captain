package build

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Target describes a single Docker image build.
type Target struct {
	Name       string
	Context    string
	Dockerfile string
	ImageTag   string
	Platform   []string
	BuildArgs  map[string]string
	CacheRef   string
}

// Builder builds Docker images. On failure, the returned error is a
// *BuildError carrying per-target failure detail when BuildKit output could be
// parsed.
type Builder interface {
	Build(ctx context.Context, target Target) error
	Bake(ctx context.Context, targets []Target) error
}

// BuildxBuilder builds images using docker buildx.
type BuildxBuilder struct {
	runner   *ExecRunner
	OnOutput func(string)
}

// NewBuildxBuilder creates a BuildxBuilder backed by the given runner.
func NewBuildxBuilder(runner *ExecRunner) *BuildxBuilder {
	return &BuildxBuilder{runner: runner}
}

// Build builds a single image target.
func (b *BuildxBuilder) Build(ctx context.Context, target Target) error {
	args := []string{"buildx", "build", "--progress=rawjson"}

	if target.ImageTag != "" {
		args = append(args, "--tag", target.ImageTag)
	}

	if target.Dockerfile != "" {
		args = append(args, "--file", target.Dockerfile)
	}

	if len(target.Platform) > 0 {
		args = append(args, "--platform", strings.Join(target.Platform, ","))
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

	dec := newProgressDecoder(b.OnOutput)
	// buildx writes --progress=rawjson to stderr, not stdout.
	stdout, stderr, err := b.runner.RunStreaming(ctx, nil, dec.HandleLine, "docker", args...)
	if err != nil {
		failures := collectBuildFailures(dec, stdout, stderr, target.Name)
		return &BuildError{Failures: failures, Err: fmt.Errorf("building %s: %w", target.Name, err)}
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

	if len(targets) == 1 {
		return b.Build(ctx, targets[0])
	}

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
		if len(t.Platform) > 0 {
			bt.Platforms = t.Platform
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

	args := []string{"buildx", "bake", "--progress=rawjson", "-f", bakePath}

	dec := newProgressDecoder(b.OnOutput)
	// buildx writes --progress=rawjson to stderr, not stdout.
	stdout, stderr, err := b.runner.RunStreaming(ctx, nil, dec.HandleLine, "docker", args...)
	if err != nil {
		failures := collectBuildFailures(dec, stdout, stderr, "")
		return &BuildError{Failures: failures, Err: fmt.Errorf("bake: %w", err)}
	}
	return nil
}

// cacheRefArgs returns the --cache-from and --cache-to values for a registry cache ref.
func cacheRefArgs(ref string) (from, to string) {
	return "type=registry,ref=" + ref, "type=registry,ref=" + ref + ",mode=max"
}

// shouldPush returns true if the image tag references a remote registry.
func shouldPush(imageTag string) bool {
	if imageTag == "" {
		return false
	}
	parts := strings.SplitN(imageTag, "/", 2)
	if len(parts) < 2 {
		return false
	}
	host := parts[0]
	return strings.Contains(host, ".") || strings.Contains(host, ":") || host == "localhost"
}
