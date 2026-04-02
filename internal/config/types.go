package config

type Config struct {
	Namespace string                    `yaml:"namespace"`
	Setup     []SetupStep              `yaml:"setup,omitempty"`
	Cluster   map[string]ClusterConfig `yaml:"cluster,omitempty"`
	Services  map[string]ServiceConfig `yaml:"services,omitempty"`
	// Deprecated: use Services. Kept for backward compat — merged into Services during load.
	Infra map[string]ServiceConfig `yaml:"infra,omitempty"`
	Apps  map[string]ServiceConfig `yaml:"apps,omitempty"`
}

type SetupStep struct {
	Name   string   `yaml:"name"`
	Check  string   `yaml:"check,omitempty"`
	Run    string   `yaml:"run"`
	Stacks []string `yaml:"stacks,omitempty"`
}

type ClusterConfig struct {
	Context  string          `yaml:"context,omitempty"`
	Provider string          `yaml:"provider,omitempty"`
	Registry *RegistryConfig `yaml:"registry,omitempty"`
	Platform []string        `yaml:"platform,omitempty"`
}

type RegistryConfig struct {
	Push            string `yaml:"push"`
	Pull            string `yaml:"pull,omitempty"`
	Server          string `yaml:"server,omitempty"`
	Username        string `yaml:"username,omitempty"`
	Password        string `yaml:"password,omitempty"`
	ImagePullSecret string `yaml:"imagePullSecret,omitempty"`
}

// PullRegistry returns the pull registry, defaulting to Push if not set.
func (r *RegistryConfig) PullRegistry() string {
	if r.Pull != "" {
		return r.Pull
	}
	return r.Push
}

// NeedsSecret returns true if the registry has credentials that need a K8s secret.
func (r *RegistryConfig) NeedsSecret() bool {
	return r.Server != "" && r.Username != "" && r.Password != ""
}

// SecretName returns the name for the imagePullSecret, defaulting to "registry-credentials".
func (r *RegistryConfig) SecretName() string {
	if r.ImagePullSecret != "" {
		return r.ImagePullSecret
	}
	return "registry-credentials"
}

type ServiceConfig struct {
	Chart      string                 `yaml:"chart"`
	Build      *BuildConfig           `yaml:"build,omitempty"`
	Disabled   bool                   `yaml:"disabled,omitempty"`
	Stacks     []string               `yaml:"stacks,omitempty"`
	Values     map[string]interface{} `yaml:"values,omitempty"`
	Exposes    map[string]string      `yaml:"exposes,omitempty"`
	References []string               `yaml:"references,omitempty"`
	Secrets    map[string]string      `yaml:"secrets,omitempty"`
}

type BuildConfig struct {
	Context    string   `yaml:"context"`
	Dockerfile string   `yaml:"dockerfile,omitempty"`
	Image      string   `yaml:"image,omitempty"`
	Target     string   `yaml:"target,omitempty"`
	Platform   []string `yaml:"platform,omitempty"`
	Watch      []string `yaml:"watch,omitempty"`
	CacheRef   string   `yaml:"cache_ref,omitempty"`
}
