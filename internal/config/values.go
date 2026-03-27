package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadStackValues looks for a values-<stack>.yaml file alongside the chart's values.yaml.
// If found, it returns the parsed values. If not found, returns nil (not an error).
func LoadStackValues(chartDir, stack string) (map[string]interface{}, error) {
	if stack == "" {
		return nil, nil
	}

	path := filepath.Join(chartDir, fmt.Sprintf("values-%s.yaml", stack))
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var vals map[string]interface{}
	if err := yaml.Unmarshal(data, &vals); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return vals, nil
}

// MergeValues deep-merges src into dst. src values override dst values.
// Maps are merged recursively; all other types are replaced.
func MergeValues(dst, src map[string]interface{}) map[string]interface{} {
	if dst == nil {
		dst = make(map[string]interface{})
	}
	for k, srcVal := range src {
		if dstVal, ok := dst[k]; ok {
			dstMap, dstIsMap := dstVal.(map[string]interface{})
			srcMap, srcIsMap := srcVal.(map[string]interface{})
			if dstIsMap && srcIsMap {
				dst[k] = MergeValues(dstMap, srcMap)
				continue
			}
		}
		dst[k] = srcVal
	}
	return dst
}
