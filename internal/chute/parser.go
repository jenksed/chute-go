package chute

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type ParseError struct {
	Source string `json:"source"`
	Error  string `json:"error"`
}

func ParseResources(root string, inventory []InventoryEntry) ([]Resource, []ParseError) {
	var resources []Resource
	var parseErrors []ParseError

	for _, entry := range inventory {
		if entry.Category != "yaml" {
			continue
		}

		fileResources, err := parseYAMLFile(physicalPath(root, entry), entry.Path)
		if err != nil {
			parseErrors = append(parseErrors, ParseError{
				Source: entry.Path,
				Error:  err.Error(),
			})
			continue
		}
		resources = append(resources, fileResources...)
	}

	return resources, parseErrors
}

func parseYAMLFile(path, source string) ([]Resource, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	var resources []Resource

	for {
		var document any
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if document == nil {
			continue
		}

		normalized := normalizeYAML(document)
		resources = append(resources, resourcesFromDocument(normalized, source)...)
	}

	return resources, nil
}

func resourcesFromDocument(document any, source string) []Resource {
	switch value := document.(type) {
	case []any:
		var resources []Resource
		for _, item := range value {
			resources = append(resources, resourcesFromDocument(item, source)...)
		}
		return resources

	case map[string]any:
		kind, _ := value["kind"].(string)
		if listKind(kind) {
			if items, ok := value["items"].([]any); ok {
				var resources []Resource
				for _, item := range items {
					resources = append(resources, resourcesFromDocument(item, source)...)
				}
				return resources
			}
		}

		if resource, ok := ResourceFromMap(value, source); ok {
			return []Resource{resource}
		}
	}

	return nil
}

func listKind(kind string) bool {
	return kind == "List" || strings.HasSuffix(kind, "List")
}

func normalizeYAML(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = normalizeYAML(item)
		}
		return out

	case map[any]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[fmt.Sprint(key)] = normalizeYAML(item)
		}
		return out

	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = normalizeYAML(item)
		}
		return out

	default:
		return value
	}
}
