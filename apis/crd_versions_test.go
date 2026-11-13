package apis

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"sigs.k8s.io/yaml"
)

// TestServedVersionsDeclareStorageDefaults guards #976: with conversion strategy
// None, every served version must declare each field the storage version
// defaults, or server-side apply through that version fails.
func TestServedVersionsDeclareStorageDefaults(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "package", "crds", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no CRDs found under package/crds")
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var crd struct {
			Spec struct {
				Versions []struct {
					Name    string `json:"name"`
					Served  bool   `json:"served"`
					Storage bool   `json:"storage"`
					Schema  struct {
						OpenAPIV3Schema map[string]any `json:"openAPIV3Schema"`
					} `json:"schema"`
				} `json:"versions"`
			} `json:"spec"`
		}
		if err := yaml.Unmarshal(raw, &crd); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		var storage map[string]any
		var storageName string
		for _, v := range crd.Spec.Versions {
			if v.Storage {
				storage, storageName = v.Schema.OpenAPIV3Schema, v.Name
			}
		}
		defaulted := defaultedPaths(storage)
		for _, v := range crd.Spec.Versions {
			if v.Storage || !v.Served {
				continue
			}
			for _, p := range defaulted {
				if !declares(v.Schema.OpenAPIV3Schema, p) {
					t.Errorf("%s: %s defaults %s, but served version %s does not declare it", filepath.Base(f), storageName, joinPath(p), v.Name)
				}
			}
		}
	}
}

// defaultedPaths returns the property paths below schema that carry a default.
// An array element step is recorded as "[]".
func defaultedPaths(schema map[string]any) [][]string {
	var out [][]string
	var walk func(s map[string]any, path []string)
	walk = func(s map[string]any, path []string) {
		if _, ok := s["default"]; ok && len(path) > 0 {
			out = append(out, append([]string(nil), path...))
		}
		props, _ := s["properties"].(map[string]any)
		for _, k := range slices.Sorted(maps.Keys(props)) {
			if child, ok := props[k].(map[string]any); ok {
				walk(child, append(path, k))
			}
		}
		if items, ok := s["items"].(map[string]any); ok {
			walk(items, append(path, "[]"))
		}
	}
	walk(schema, nil)
	return out
}

func declares(schema map[string]any, path []string) bool {
	s := schema
	for _, step := range path {
		var next map[string]any
		if step == "[]" {
			next, _ = s["items"].(map[string]any)
		} else {
			props, _ := s["properties"].(map[string]any)
			next, _ = props[step].(map[string]any)
		}
		if next == nil {
			// An object that preserves unknown fields accepts any child.
			preserve, _ := s["x-kubernetes-preserve-unknown-fields"].(bool)
			return preserve
		}
		s = next
	}
	return true
}

func joinPath(p []string) string {
	out := ""
	for _, s := range p {
		if s == "[]" {
			out += "[]"
			continue
		}
		out += "." + s
	}
	return out
}
