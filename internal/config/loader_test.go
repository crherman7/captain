package config

import (
	"strings"
	"testing"
)

func TestLoadFromReader_Valid(t *testing.T) {
	input := `
cluster:
  local:
    context: k3d-test
    namespace: test
services:
  postgres:
    chart: deploy/infra/postgres
    exposes:
      DATABASE_URL: "postgresql://user:pass@{name}:5432/db"
  api:
    chart: deploy/apps/api
    references: [postgres]
    secrets:
      API_KEY: ${env:API_KEY}
`
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	local := cfg.GetCluster("local")
	if local == nil || local.Namespace != "test" {
		t.Errorf("cluster local namespace = %q, want %q", local.Namespace, "test")
	}
	if len(cfg.Services) != 2 {
		t.Errorf("services count = %d, want 2", len(cfg.Services))
	}
	if cfg.Services["api"].References[0] != "postgres" {
		t.Errorf("api references = %v, want [postgres]", cfg.Services["api"].References)
	}
}

func TestLoadFromReader_BackwardCompat(t *testing.T) {
	input := `
infra:
  postgres:
    chart: deploy/infra/postgres
apps:
  api:
    chart: deploy/apps/api
    references: [postgres]
`
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if len(cfg.Services) != 2 {
		t.Errorf("services count = %d, want 2 (merged from infra+apps)", len(cfg.Services))
	}
}

func TestLoadFromReader_MissingNamespace(t *testing.T) {
	input := `
cluster:
  local:
    context: k3d-test
services:
  redis:
    chart: deploy/infra/redis
`
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for missing namespace on cluster")
	}
}

func TestLoadFromReader_MissingChart(t *testing.T) {
	input := `
services:
  redis: {}
`
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected validation error for missing chart")
	}
}

func TestValidate_DanglingReference(t *testing.T) {
	input := `
services:
  api:
    chart: deploy/apps/api
    references: [postgres]
`
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	err = cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for dangling reference")
	}
	if !strings.Contains(err.Error(), "unknown service") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidate_SetupSteps(t *testing.T) {
	input := `
setup:
  - name: create-cluster
    check: k3d cluster list | grep -q test
    run: k3d cluster create test
  - name: wait-ready
    run: kubectl wait --for=condition=ready nodes --all
services:
  redis:
    chart: deploy/infra/redis
`
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if len(cfg.Setup) != 2 {
		t.Errorf("setup count = %d, want 2", len(cfg.Setup))
	}
}

func TestValidate_SetupMissingName(t *testing.T) {
	input := `
setup:
  - run: echo hello
`
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	err = cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for missing setup name")
	}
}

func TestValidate_SetupMissingRun(t *testing.T) {
	input := `
setup:
  - name: broken
    check: true
`
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	err = cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for missing setup run")
	}
}

func TestValidate_SetupDuplicateName(t *testing.T) {
	input := `
setup:
  - name: step1
    run: echo a
  - name: step1
    run: echo b
`
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	err = cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for duplicate setup name")
	}
}

func TestValidate_DisabledService(t *testing.T) {
	input := `
services:
  debug:
    chart: deploy/debug
    disabled: true
  api:
    chart: deploy/apps/api
`
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if !cfg.Services["debug"].Disabled {
		t.Error("expected debug to be disabled")
	}
}

func TestGetCluster(t *testing.T) {
	input := `
cluster:
  local:
    context: k3d-test
    registry:
      push: localhost:5001
      pull: test-registry:5000
  production:
    context: prod-cluster
    registry:
      push: ghcr.io/myorg
services:
  api:
    chart: deploy/apps/api
`
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	local := cfg.GetCluster("local")
	if local == nil {
		t.Fatal("expected local cluster config")
	}
	if local.Context != "k3d-test" {
		t.Errorf("context = %q, want %q", local.Context, "k3d-test")
	}
	if local.Registry.Push != "localhost:5001" {
		t.Errorf("push = %q, want %q", local.Registry.Push, "localhost:5001")
	}
	if local.Registry.PullRegistry() != "test-registry:5000" {
		t.Errorf("pull = %q, want %q", local.Registry.PullRegistry(), "test-registry:5000")
	}

	prod := cfg.GetCluster("production")
	if prod == nil {
		t.Fatal("expected production cluster config")
	}
	// Pull defaults to Push when not set
	if prod.Registry.PullRegistry() != "ghcr.io/myorg" {
		t.Errorf("pull = %q, want %q", prod.Registry.PullRegistry(), "ghcr.io/myorg")
	}

	// Unknown stack
	if cfg.GetCluster("staging") != nil {
		t.Error("expected nil for unknown stack")
	}
}

func TestGetCluster_SingleAutoSelect(t *testing.T) {
	input := `
cluster:
  local:
    context: k3d-test
services:
  api:
    chart: deploy/apps/api
`
	cfg, err := LoadFromReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Empty stack with single cluster should auto-select
	c := cfg.GetCluster("")
	if c == nil {
		t.Fatal("expected auto-selected cluster")
	}
	if c.Context != "k3d-test" {
		t.Errorf("context = %q, want %q", c.Context, "k3d-test")
	}
}
