package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/christopherherman/captain/internal/build"
	"github.com/christopherherman/captain/internal/config"
	"github.com/christopherherman/captain/internal/deploy"
	"github.com/christopherherman/captain/internal/graph"
	"github.com/christopherherman/captain/internal/resolver"
	"github.com/christopherherman/captain/internal/state"
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
	EnvSecretData map[string]string // merged exposes + secrets for <name>-env K8s secret
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
	if svc.Disabled {
		return false
	}
	if p.Stack == "" || len(svc.Stacks) == 0 {
		return true
	}
	for _, s := range svc.Stacks {
		if s == p.Stack {
			return true
		}
	}
	return false
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
			Namespace:   p.Config.Namespace,
			Exposed:     make(map[string]string),
		}

		// Merge exposes from referenced services
		for _, ref := range svc.References {
			if exposed, ok := exposedValues[ref]; ok {
				for k, v := range exposed {
					ctx.Exposed[k] = v
				}
			}
		}

		// Start with captain.yaml values
		resolvedValues := make(map[string]interface{})
		if svc.Values != nil {
			resolvedValues, err = p.Resolver.ResolveMap(svc.Values, ctx)
			if err != nil {
				return nil, fmt.Errorf("service %q values: %w", name, err)
			}
		}

		// Merge per-stack values file (values-<stack>.yaml) if it exists
		if p.Stack != "" {
			stackVals, err := config.LoadStackValues(svc.Chart, p.Stack)
			if err != nil {
				return nil, fmt.Errorf("service %q stack values: %w", name, err)
			}
			if stackVals != nil {
				// Resolve env vars in stack values too
				resolvedStackVals, err := p.Resolver.ResolveMap(stackVals, ctx)
				if err != nil {
					return nil, fmt.Errorf("service %q stack values resolve: %w", name, err)
				}
				resolvedValues = config.MergeValues(resolvedValues, resolvedStackVals)
			}
		}

		// Inject referenced exposes into values under "env" key
		if len(ctx.Exposed) > 0 {
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

		// Resolve and inject secrets
		if len(svc.Secrets) > 0 {
			resolvedSecrets, err := p.Resolver.ResolveStringMap(svc.Secrets, ctx)
			if err != nil {
				return nil, fmt.Errorf("service %q secrets: %w", name, err)
			}
			secretsMap := make(map[string]interface{}, len(resolvedSecrets))
			for k, v := range resolvedSecrets {
				secretsMap[k] = v
			}
			resolvedValues["secrets"] = secretsMap
		}

		// Build env secret data (exposes from references + service secrets)
		envSecretData := make(map[string]string)
		for k, v := range ctx.Exposed {
			envSecretData[k] = v
		}
		if len(svc.Secrets) > 0 {
			resolvedSecrets, err := p.Resolver.ResolveStringMap(svc.Secrets, ctx)
			if err != nil {
				return nil, fmt.Errorf("service %q secrets: %w", name, err)
			}
			for k, v := range resolvedSecrets {
				envSecretData[k] = v
			}
		}

		// Inject imagePullSecrets if registry has credentials
		if secretName := p.imagePullSecretName(); secretName != "" {
			resolvedValues["imagePullSecrets"] = []interface{}{secretName}
		}

		// Resolve exposes and store for downstream
		if len(svc.Exposes) > 0 {
			resolvedExposes, err := p.Resolver.ResolveStringMap(svc.Exposes, ctx)
			if err != nil {
				return nil, fmt.Errorf("service %q exposes: %w", name, err)
			}
			exposedValues[name] = resolvedExposes
		}

		// Compute hashes and image tag
		hasBuild := svc.Build != nil
		var buildTarget build.Target
		var buildHash string

		if hasBuild {
			buildHash, err = state.HashBuildContext(svc.Build.Context, svc.Build.Dockerfile, svc.Build.Watch)
			if err != nil {
				return nil, fmt.Errorf("service %q build hash: %w", name, err)
			}

			imageTag := buildHash[:12]

			// Determine push image ref
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
				Platform:   svc.Build.Platform,
			}

			// Determine pull image repository
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

			// Auto-inject image.tag and image.repository
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

		hash, err := state.ComputeHash(resolvedValues, buildHash)
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

// Execute runs the full build + deploy pipeline.
func (p *Pipeline) Execute(ctx context.Context, actions []PlannedAction, spin *Spinner) error {
	// Ensure registry secret exists before deploying
	if p.Cluster != nil && p.Cluster.Registry != nil && p.Cluster.Registry.NeedsSecret() {
		kubeCtx := ""
		if p.Cluster != nil {
			kubeCtx = p.Cluster.Context
		}
		spin.Start("registry", "creating secret...")
		secretName, err := deploy.EnsureRegistrySecret(ctx, p.Cluster.Registry, p.Config.Namespace, kubeCtx)
		if err != nil {
			spin.Stop("✗", "failed")
			return fmt.Errorf("registry secret: %w", err)
		}
		spin.Stop("✔", secretName)
	}

	// Collect all build targets and bake them in parallel
	var buildTargets []build.Target
	var buildNames []string
	for _, action := range actions {
		if action.NeedsBuild {
			buildTargets = append(buildTargets, action.BuildTarget)
			buildNames = append(buildNames, action.ServiceName)
		}
	}

	if len(buildTargets) > 0 {
		label := strings.Join(buildNames, ", ")
		spin.Start("build", fmt.Sprintf("building %d services (%s)...", len(buildTargets), label))
		if b, ok := p.Builder.(*build.BuildxBuilder); ok {
			b.OnOutput = func(msg string) { spin.Update("building " + msg) }
		}
		if err := p.Builder.Bake(ctx, buildTargets); err != nil {
			spin.Stop("✗", "build failed")
			return fmt.Errorf("building: %w", err)
		}
		if b, ok := p.Builder.(*build.BuildxBuilder); ok {
			b.OnOutput = nil
		}
		spin.Stop("✔", fmt.Sprintf("built %d services", len(buildTargets)))
	}

	// Deploy services in order
	for _, action := range actions {
		if action.Status == StatusUnchanged {
			spin.Skip(action.ServiceName, "unchanged")
			continue
		}

		// Always create <name>-env secret — charts expect it to exist
		{
			kubeCtx := ""
			if p.Cluster != nil {
				kubeCtx = p.Cluster.Context
			}
			if err := deploy.EnsureAppSecret(ctx, action.ServiceName, p.Config.Namespace, kubeCtx, action.EnvSecretData); err != nil {
				spin.Stop("✗", "secret failed")
				return fmt.Errorf("creating secret for %s: %w", action.ServiceName, err)
			}
		}

		svc := p.Config.Services[action.ServiceName]
		rel := deploy.Release{
			Name:      action.ServiceName,
			Namespace: p.Config.Namespace,
			Chart:     svc.Chart,
			Values:    action.Values,
		}

		spin.Start(action.ServiceName, "deploying...")
		if d, ok := p.Deployer.(*deploy.HelmDeployer); ok {
			d.OnOutput = func(msg string) { spin.Update("deploying " + msg) }
		}
		if err := p.Deployer.Deploy(ctx, rel); err != nil {
			spin.Stop("✗", "deploy failed")
			return fmt.Errorf("deploying %s: %w", action.ServiceName, err)
		}
		if d, ok := p.Deployer.(*deploy.HelmDeployer); ok {
			d.OnOutput = nil
		}
		spin.Stop("✔", "deployed")

		// Update state
		p.State.Update(action.ServiceName, state.ServiceState{
			Hash:       action.Hash,
			BuildHash:  action.BuildHash,
			DeployedAt: time.Now(),
			ImageTag:   action.BuildTarget.ImageTag,
		})
	}

	return nil
}
