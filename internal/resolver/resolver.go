package resolver

import (
	"fmt"
	"regexp"
)

var (
	envPattern  = regexp.MustCompile(`\$\{env:([^}]+)\}`)
	namePattern = regexp.MustCompile(`\{name\}`)
)

// EnvFunc looks up an environment variable. Returns value and whether it was found.
type EnvFunc func(string) (string, bool)

type ResolveContext struct {
	ServiceName string
	Namespace   string
	Exposed     map[string]string // merged exposes from referenced infra
}

type Resolver struct {
	env EnvFunc
}

func New(env EnvFunc) *Resolver {
	return &Resolver{env: env}
}

func (r *Resolver) ResolveString(s string, ctx ResolveContext) (string, error) {
	var resolveErr error

	result := envPattern.ReplaceAllStringFunc(s, func(match string) string {
		if resolveErr != nil {
			return match
		}
		sub := envPattern.FindStringSubmatch(match)
		val, ok := r.env(sub[1])
		if !ok {
			resolveErr = fmt.Errorf("environment variable %q not set", sub[1])
			return match
		}
		return val
	})
	if resolveErr != nil {
		return "", resolveErr
	}

	result = namePattern.ReplaceAllString(result, ctx.ServiceName)

	return result, nil
}

func (r *Resolver) ResolveMap(m map[string]interface{}, ctx ResolveContext) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		resolved, err := r.resolveValue(v, ctx)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", k, err)
		}
		out[k] = resolved
	}
	return out, nil
}

func (r *Resolver) ResolveStringMap(m map[string]string, ctx ResolveContext) (map[string]string, error) {
	out := make(map[string]string, len(m))
	for k, v := range m {
		resolved, err := r.ResolveString(v, ctx)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", k, err)
		}
		out[k] = resolved
	}
	return out, nil
}

func (r *Resolver) resolveValue(v interface{}, ctx ResolveContext) (interface{}, error) {
	switch val := v.(type) {
	case string:
		return r.ResolveString(val, ctx)
	case map[string]interface{}:
		return r.ResolveMap(val, ctx)
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, item := range val {
			resolved, err := r.resolveValue(item, ctx)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	default:
		return v, nil
	}
}
