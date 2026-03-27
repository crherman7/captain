package config

import (
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"
)

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening config: %w", err)
	}
	defer func() { _ = f.Close() }()
	return LoadFromReader(f)
}

func LoadFromReader(r io.Reader) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(r)
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.merge()
	return &cfg, nil
}

// merge combines deprecated Infra/Apps into Services for backward compat.
func (c *Config) merge() {
	if c.Services == nil {
		c.Services = make(map[string]ServiceConfig)
	}
	for k, v := range c.Infra {
		c.Services[k] = v
	}
	for k, v := range c.Apps {
		c.Services[k] = v
	}
}

func (c *Config) Validate() error {
	if c.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}

	// Validate setup steps
	setupNames := make(map[string]bool)
	for i, step := range c.Setup {
		if step.Name == "" {
			return fmt.Errorf("setup step %d: name is required", i)
		}
		if step.Run == "" {
			return fmt.Errorf("setup step %q: run is required", step.Name)
		}
		if setupNames[step.Name] {
			return fmt.Errorf("duplicate setup step name %q", step.Name)
		}
		setupNames[step.Name] = true
	}

	// Validate services
	for name, svc := range c.Services {
		if svc.Chart == "" {
			return fmt.Errorf("service %q: chart is required", name)
		}
		for _, ref := range svc.References {
			if _, ok := c.Services[ref]; !ok {
				return fmt.Errorf("service %q references unknown service %q", name, ref)
			}
		}
	}

	return nil
}

// GetCluster returns the cluster config for the given stack.
// If stack is empty and there is exactly one cluster, it returns that one.
// Returns nil if no matching cluster is found.
func (c *Config) GetCluster(stack string) *ClusterConfig {
	if stack != "" {
		if cc, ok := c.Cluster[stack]; ok {
			return &cc
		}
		return nil
	}
	if len(c.Cluster) == 1 {
		for _, cc := range c.Cluster {
			return &cc
		}
	}
	return nil
}
