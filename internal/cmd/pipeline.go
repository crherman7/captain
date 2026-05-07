package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/crherman7/captain/internal/build"
	"github.com/crherman7/captain/internal/config"
	"github.com/crherman7/captain/internal/deploy"
	"github.com/crherman7/captain/internal/graph"
	"github.com/crherman7/captain/internal/resolver"
	"github.com/crherman7/captain/internal/state"
)

type ActionStatus int

const (
	StatusUnchanged ActionStatus = iota
	StatusNew
	StatusChanged
)

func (s ActionStatus) String() string {
	switch s {
	case StatusUnchanged:
		return "unchanged"
	case StatusNew:
		return "new"
	case StatusChanged:
		return "changed"
	default:
		return "unknown"
	}
}

type PlannedAction struct {
	ServiceName   string
	Status        ActionStatus
	Hash          string
	BuildHash     string
	Values        map[string]interface{}
	EnvSecretData map[string]string
	HasBuild      bool
	NeedsBuild    bool
	BuildTarget   build.Target
}

type Pipeline struct {
	Config   *config.Config
	Cluster  *config.ClusterConfig
	State    *state.State
	Resolver *resolver.Resolver
	Builder  build.Builder
	Deployer deploy.Deployer
	Stack    string
}

func NewPipeline(cfg *config.Config, cluster *config.ClusterConfig, st *state.State, builder build.Builder, deployer deploy.Deployer, stack string) *Pipeline {
	return &Pipeline{
		Config:   cfg,
		Cluster:  cluster,
		State:    st,
		Resolver: resolver.New(os.LookupEnv),
		Builder:  builder,
		Deployer: deployer,
		Stack:    stack,
	}
}

func (p *Pipeline) buildGraph() (*graph.Graph, error) {
	g := graph.New()

	for name, svc := range p.Config.Services {
		if !p.includeService(svc) {
			continue
		}
		g.AddNode(name)
	}

	for name, svc := range p.Config.Services {
		if !p.includeService(svc) {
			continue
		}
		for _, ref := range svc.References {
			if err := g.AddEdge(name, ref); err != nil {
				return nil, fmt.Errorf("service %q: %w", name, err)
			}
		}
	}

	return g, nil
}

func (p *Pipeline) includeService(svc config.ServiceConfig) bool {
	return includeService(svc, p.Stack)
}

func (p *Pipeline) pushRegistry() string {
	if p.Cluster != nil && p.Cluster.Registry != nil {
		return p.Cluster.Registry.Push
	}
	return ""
}

func (p *Pipeline) pullRegistry() string {
	if p.Cluster != nil && p.Cluster.Registry != nil {
		return p.Cluster.Registry.PullRegistry()
	}
	return ""
}

func (p *Pipeline) imagePullSecretName() string {
	if p.Cluster != nil && p.Cluster.Registry != nil && p.Cluster.Registry.NeedsSecret() {
		return p.Cluster.Registry.SecretName()
	}
	return ""
}

func (p *Pipeline) kubeContext() string {
	if p.Cluster != nil {
		return p.Cluster.Context
	}
	return ""
}

func (p *Pipeline) namespace() string {
	if p.Cluster != nil {
		return p.Cluster.Namespace
	}
	return ""
}

func (p *Pipeline) buildPlatform(svcPlatform []string) []string {
	if len(svcPlatform) > 0 {
		return svcPlatform
	}
	if p.Cluster != nil && len(p.Cluster.Platform) > 0 {
		return p.Cluster.Platform
	}
	if env := os.Getenv("DOCKER_DEFAULT_PLATFORM"); env != "" {
		return []string{env}
	}
	return nil
}

// Plan resolves all services and computes what needs to change.
func (p *Pipeline) Plan() ([]PlannedAction, error) {
	g, err := p.buildGraph()
	if err != nil {
		return nil, err
	}

	sorted, err := g.Sort()
	if err != nil {
		return nil, err
	}

	exposedValues := make(map[string]map[string]string)
	var actions []PlannedAction

	for _, name := range sorted {
		svc := p.Config.Services[name]
		ctx := resolver.ResolveContext{
			ServiceName: name,
			Namespace:   p.namespace(),
			Exposed:     make(map[string]string),
		}

		for _, ref := range svc.References {
			if exposed, ok := exposedValues[ref]; ok {
				for k, v := range exposed {
					ctx.Exposed[k] = v
				}
			}
		}

		resolvedValues := make(map[string]interface{})
		if svc.Values != nil {
			resolvedValues, err = p.Resolver.ResolveMap(svc.Values, ctx)
			if err != nil {
				return nil, fmt.Errorf("service %q values: %w", name, err)
			}
		}

		if p.Stack != "" {
			stackVals, err := config.LoadStackValues(svc.Chart, p.Stack)
			if err != nil {
				return nil, fmt.Errorf("service %q stack values: %w", name, err)
			}
			if stackVals != nil {
				resolvedStackVals, err := p.Resolver.ResolveMap(stackVals, ctx)
				if err != nil {
					return nil, fmt.Errorf("service %q stack values resolve: %w", name, err)
				}
				resolvedValues = config.MergeValues(resolvedValues, resolvedStackVals)
			}
		}

		if len(ctx.Exposed) > 0 && svc.ShouldInjectEnv() {
			envMap := make(map[string]interface{})
			if existing, ok := resolvedValues["env"]; ok {
				if m, ok := existing.(map[string]interface{}); ok {
					for k, v := range m {
						envMap[k] = v
					}
				}
			}
			for k, v := range ctx.Exposed {
				envMap[k] = v
			}
			resolvedValues["env"] = envMap
		}

		envSecretData := make(map[string]string)
		for k, v := range ctx.Exposed {
			envSecretData[k] = v
		}
		if len(svc.Secrets) > 0 {
			resolvedSecrets, err := p.Resolver.ResolveStringMap(svc.Secrets, ctx)
			if err != nil {
				return nil, fmt.Errorf("service %q secrets: %w", name, err)
			}
			secretsMap := make(map[string]interface{}, len(resolvedSecrets))
			for k, v := range resolvedSecrets {
				envSecretData[k] = v
				secretsMap[k] = v
			}
			resolvedValues["secrets"] = secretsMap
		}

		if secretName := p.imagePullSecretName(); secretName != "" {
			resolvedValues["imagePullSecrets"] = []interface{}{secretName}
		}

		if len(svc.Exposes) > 0 {
			resolvedExposes, err := p.Resolver.ResolveStringMap(svc.Exposes, ctx)
			if err != nil {
				return nil, fmt.Errorf("service %q exposes: %w", name, err)
			}
			exposedValues[name] = resolvedExposes
		}

		hasBuild := svc.Build != nil
		var buildTarget build.Target
		var buildHash string

		if hasBuild {
			buildHash, err = state.HashBuildContext(svc.Build.Context, svc.Build.Dockerfile, svc.Build.Watch)
			if err != nil {
				return nil, fmt.Errorf("service %q build hash: %w", name, err)
			}

			imageTag := buildHash[:12]

			var pushRef string
			if svc.Build.Image != "" {
				pushRef = svc.Build.Image
				if idx := strings.LastIndex(pushRef, ":"); idx > 0 {
					slashIdx := strings.LastIndex(pushRef, "/")
					if idx > slashIdx {
						pushRef = pushRef[:idx]
					}
				}
			} else if reg := p.pushRegistry(); reg != "" {
				pushRef = reg + "/" + name
			} else {
				pushRef = name
			}
			pushRef += ":" + imageTag

			buildTarget = build.Target{
				Name:       name,
				Context:    svc.Build.Context,
				Dockerfile: svc.Build.Dockerfile,
				ImageTag:   pushRef,
				Platform:   p.buildPlatform(svc.Build.Platform),
				CacheRef:   svc.Build.CacheRef,
			}

			pullRepo := ""
			if img, ok := resolvedValues["image"]; ok {
				if m, ok := img.(map[string]interface{}); ok {
					if r, ok := m["repository"].(string); ok {
						pullRepo = r
					}
				}
			}
			if pullRepo == "" {
				if reg := p.pullRegistry(); reg != "" {
					pullRepo = reg + "/" + name
				} else {
					pullRepo = name
				}
			}

			imageMap := make(map[string]interface{})
			if existing, ok := resolvedValues["image"]; ok {
				if m, ok := existing.(map[string]interface{}); ok {
					for k, v := range m {
						imageMap[k] = v
					}
				}
			}
			imageMap["tag"] = imageTag
			imageMap["repository"] = pullRepo
			resolvedValues["image"] = imageMap
		}

		chartHash, err := state.HashChart(svc.Chart)
		if err != nil {
			return nil, fmt.Errorf("service %q chart hash: %w", name, err)
		}

		hash, err := state.ComputeHash(resolvedValues, buildHash, chartHash)
		if err != nil {
			return nil, fmt.Errorf("service %q hash: %w", name, err)
		}

		status := StatusUnchanged
		needsBuild := false
		if _, exists := p.State.Services[name]; !exists {
			status = StatusNew
			needsBuild = hasBuild
		} else if p.State.HasChanged(name, hash) {
			status = StatusChanged
			if hasBuild {
				prev, _ := p.State.Get(name)
				needsBuild = prev.BuildHash != buildHash
			}
		}

		actions = append(actions, PlannedAction{
			ServiceName:   name,
			Status:        status,
			Hash:          hash,
			BuildHash:     buildHash,
			Values:        resolvedValues,
			EnvSecretData: envSecretData,
			HasBuild:      hasBuild,
			NeedsBuild:    needsBuild,
			BuildTarget:   buildTarget,
		})
	}

	return actions, nil
}

// Execute runs the full build + deploy pipeline with parallel layer deploys.
func (p *Pipeline) Execute(ctx context.Context, actions []PlannedAction, ui UI) error {
	// Ensure registry secret exists before deploying
	if p.Cluster != nil && p.Cluster.Registry != nil && p.Cluster.Registry.NeedsSecret() {
		ui.ServiceStart("registry", "creating secret...")
		secretName, err := deploy.EnsureRegistrySecret(ctx, p.Cluster.Registry, p.namespace(), p.kubeContext())
		if err != nil {
			ui.ServiceDone("registry", "✗", "failed")
			return fmt.Errorf("registry secret: %w", err)
		}
		ui.ServiceDone("registry", "✔", secretName)
	}

	// Bake all builds in parallel with per-target status
	var buildTargets []build.Target
	for _, action := range actions {
		if action.NeedsBuild {
			buildTargets = append(buildTargets, action.BuildTarget)
		}
	}

	if len(buildTargets) > 0 {
		ui.Header("Build")
		for _, t := range buildTargets {
			ui.ServiceStart("build/"+t.Name, "building...")
		}

		if b, ok := p.Builder.(*build.BuildxBuilder); ok {
			b.OnOutput = func(msg string) {
				target, step := build.ParseTargetMessage(msg)
				if target != "" {
					ui.ServiceUpdate("build/"+target, "building "+step)
				}
			}
		}

		if err := p.Builder.Bake(ctx, buildTargets); err != nil {
			reportBuildFailures(ui, buildTargets, err, "build/")
			return fmt.Errorf("building: %w", err)
		}

		if b, ok := p.Builder.(*build.BuildxBuilder); ok {
			b.OnOutput = nil
		}

		for _, t := range buildTargets {
			ui.ServiceDone("build/"+t.Name, "✔", "built")
		}
	}

	// Build action lookup and graph for layer-based parallel deploys
	actionMap := make(map[string]*PlannedAction, len(actions))
	for i := range actions {
		actionMap[actions[i].ServiceName] = &actions[i]
	}

	g, err := p.buildGraph()
	if err != nil {
		return err
	}

	layers, err := g.Layers()
	if err != nil {
		return err
	}

	ui.Header("Deploy")

	// Deploy layer by layer — services within a layer run in parallel
	for _, layer := range layers {
		eg, layerCtx := errgroup.WithContext(ctx)

		for _, name := range layer {
			action, ok := actionMap[name]
			if !ok {
				continue
			}

			if action.Status == StatusUnchanged {
				ui.ServiceSkip("deploy/"+action.ServiceName, "unchanged")
				continue
			}

			eg.Go(func() error {
				key := "deploy/" + action.ServiceName

				// Create <name>-env secret
				if err := deploy.EnsureAppSecret(layerCtx, action.ServiceName, p.namespace(), p.kubeContext(), action.EnvSecretData); err != nil {
					ui.ServiceDone(key, "✗", "secret failed")
					return fmt.Errorf("creating secret for %s: %w", action.ServiceName, err)
				}

				svc := p.Config.Services[action.ServiceName]
				rel := deploy.Release{
					Name:      action.ServiceName,
					Namespace: p.namespace(),
					Chart:     svc.Chart,
					Values:    action.Values,
				}

				ui.ServiceStart(key, "deploying...")

				deployer := deploy.NewHelmDeployer(p.kubeContext())
				deployer.OnOutput = func(msg string) {
					ui.ServiceUpdate(key, "deploying "+msg)
				}

				if err := deployer.Deploy(layerCtx, rel); err != nil {
					ui.ServiceDone(key, "✗", "deploy failed")
					return fmt.Errorf("deploying %s: %w", action.ServiceName, err)
				}

				ui.ServiceDone(key, "✔", "deployed")

				p.State.Update(action.ServiceName, state.ServiceState{
					Hash:       action.Hash,
					BuildHash:  action.BuildHash,
					DeployedAt: time.Now(),
					ImageTag:   action.BuildTarget.ImageTag,
				})

				return nil
			})
		}

		if err := eg.Wait(); err != nil {
			return err
		}
	}

	return nil
}

// reportBuildFailures attributes per-target failure messages to the UI.
//
// Three cases:
//   - At least one failure names a real target → that target gets ✗ with the
//     specific error; the other targets were cancelled by the bake when the
//     sibling failed.
//   - Failures exist but none name a real target (synthetic stderr-tail
//     fallback for daemon/auth/push errors) → all targets are marked ✗ with
//     a generic "build failed", and the global failure block is rendered by
//     the UI's Error path.
//   - No failures parsed at all → all targets ✗ "build failed".
func reportBuildFailures(ui UI, targets []build.Target, err error, prefix string) {
	failed := map[string]*build.BuildFailure{}
	var be *build.BuildError
	if errors.As(err, &be) {
		for i := range be.Failures {
			if be.Failures[i].Target == "" {
				continue
			}
			failed[be.Failures[i].Target] = &be.Failures[i]
		}
	}
	hasAttributedTarget := false
	for _, t := range targets {
		if _, ok := failed[t.Name]; ok {
			hasAttributedTarget = true
			break
		}
	}
	for _, t := range targets {
		if f := failed[t.Name]; f != nil {
			msg := f.Error
			if msg == "" {
				msg = "build failed"
			}
			ui.ServiceDone(prefix+t.Name, "✗", msg)
		} else if hasAttributedTarget {
			ui.ServiceDone(prefix+t.Name, "-", "cancelled")
		} else {
			ui.ServiceDone(prefix+t.Name, "✗", "build failed")
		}
	}
}
