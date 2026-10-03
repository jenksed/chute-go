package chute

import (
	"fmt"
	"strings"
)

type Resource struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Namespace  string         `json:"namespace,omitempty"`
	Name       string         `json:"name"`
	UID        string         `json:"uid,omitempty"`
	Source     string         `json:"source"`
	Data       map[string]any `json:"-"`
}

func ResourceFromMap(data map[string]any, source string) (Resource, bool) {
	kind, _ := data["kind"].(string)
	metadata, _ := data["metadata"].(map[string]any)
	name, _ := metadata["name"].(string)
	if kind == "" || name == "" {
		return Resource{}, false
	}

	apiVersion, _ := data["apiVersion"].(string)
	namespace, _ := metadata["namespace"].(string)
	uid, _ := metadata["uid"].(string)

	return Resource{
		APIVersion: apiVersion,
		Kind:       kind,
		Namespace:  namespace,
		Name:       name,
		UID:        uid,
		Source:     source,
		Data:       data,
	}, true
}

func (r Resource) Longhorn() bool {
	return strings.HasPrefix(r.APIVersion, "longhorn.io/")
}

func (r Resource) Summary() map[string]any {
	return map[string]any{
		"apiVersion": r.APIVersion,
		"kind":       r.Kind,
		"namespace":  emptyToNil(r.Namespace),
		"name":       r.Name,
		"uid":        emptyToNil(r.UID),
		"source":     r.Source,
	}
}

func (r Resource) SourceRef() map[string]any {
	return map[string]any{
		"kind":      r.Kind,
		"namespace": emptyToNil(r.Namespace),
		"name":      r.Name,
		"source":    r.Source,
	}
}

func emptyToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nestedValue(data map[string]any, path ...string) any {
	var current any = data
	for _, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = m[key]
		if !ok {
			return nil
		}
	}
	return current
}

func nestedString(data map[string]any, path ...string) string {
	value := nestedValue(data, path...)
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return ""
	}
}

func nestedMap(data map[string]any, path ...string) map[string]any {
	value := nestedValue(data, path...)
	m, _ := value.(map[string]any)
	return m
}

func nestedSlice(data map[string]any, path ...string) []any {
	value := nestedValue(data, path...)
	s, _ := value.([]any)
	return s
}
