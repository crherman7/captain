# captain

Captain is a Go CLI for orchestrating Docker builds and Helm-based Kubernetes deployments from a single `captain.yaml` file.

It is designed for repos that have multiple services, shared infrastructure, stack-specific clusters, and a small amount of deployment wiring that should live in code instead of shell scripts.

## What It Does

- Runs setup steps before deployment
- Builds Docker images with `docker buildx`
- Computes change plans so unchanged services can be skipped
- Resolves service references and exposed outputs
- Deploys services with the Helm SDK
- Creates or updates Kubernetes secrets needed for app env and registry auth

## Commands

```bash
captain build
captain deploy
captain destroy
captain diff
captain outputs
captain setup
```

Global flags:

```bash
captain --config captain.yaml --stack local --state .captain-state.local.json
```

## Install

Build or install from the standard Go entrypoint:

```bash
go build ./cmd/captain
go install ./cmd/captain
```

## Requirements

Captain assumes you already have the platform tooling for your workflow:

- Docker with `buildx`
- Access to a Kubernetes cluster
- A valid kubeconfig
- Helm charts for the services you deploy

If your setup steps provision local infrastructure, you may also need tools such as `kubectl`, `k3d`, or `curl`.

> [!NOTE]
> Captain uses the Helm SDK directly, but your project still needs real charts and a reachable Kubernetes cluster.

## Quick Start

The repository includes a runnable example in [`examples/hello-world`](./examples/hello-world).

```bash
cd examples/hello-world
set -a && source .env && set +a
captain deploy -s local
```

Typical flow:

1. Define a `captain.yaml`
2. Add any required setup steps
3. Point each service at a Helm chart
4. Add build config for services that need images
5. Run `captain diff` or `captain deploy`

## Configuration

Captain uses a single YAML file, typically named `captain.yaml`.

Example:

```yaml
namespace: hello

cluster:
  local:
    context: k3d-hello
    registry:
      push: localhost:5001
      pull: hello-registry:5000

setup:
  - name: k3d-cluster
    stacks: [local]
    check: k3d cluster list -o json | grep -q hello
    run: k3d cluster create hello --registry-create hello-registry:0.0.0.0:5001 --port 80:80@loadbalancer --wait

services:
  postgres:
    chart: deploy/infra/postgres
    values:
      postgres:
        auth:
          password: ${env:DB_PASSWORD}
    exposes:
      DATABASE_URL: "postgresql://hello:${env:DB_PASSWORD}@{name}:5432/hello?sslmode=disable"

  api:
    chart: deploy/apps/api
    build:
      context: services/api
      platform: linux/arm64
    references: [postgres]
```

Important fields:

- `namespace`: target Kubernetes namespace
- `cluster.<stack>`: kube context and registry settings for a stack
- `setup`: shell-based preflight or provisioning steps
- `services.<name>.chart`: Helm chart path for a service
- `services.<name>.build`: Docker build configuration
- `services.<name>.references`: service dependency edges
- `services.<name>.exposes`: computed outputs other services can consume
- `services.<name>.secrets`: secret values resolved at deploy time

## Stack-Aware Deployments

Stacks let you target different clusters or registry settings from the same config:

```bash
captain deploy -s local
captain deploy -s production
```

State is also stack-aware. When you pass `--stack`, Captain scopes the state file to `.captain-state.<stack>.json`.

## Typical Workflow

Use `diff` to preview changes:

```bash
captain diff -s local
```

Build only:

```bash
captain build -s local
```

Deploy:

```bash
captain deploy -s local
```

Inspect resolved outputs:

```bash
captain outputs -s local
```

Destroy deployed services:

```bash
captain destroy -s local
```

## Project Layout

```text
cmd/captain/        installable CLI entrypoint
internal/cmd/       Cobra commands and terminal UI
internal/build/     docker buildx orchestration
internal/config/    config loading and validation
internal/deploy/    Helm and Kubernetes deploy helpers
internal/graph/     dependency graph ordering
internal/resolver/  exposes/reference resolution
internal/setup/     setup step runner
internal/state/     deploy state and change detection
examples/           example projects and configs
```

## Testing

Run the full test suite:

```bash
go test ./...
```

Build the installable binary:

```bash
go build ./cmd/captain
```
