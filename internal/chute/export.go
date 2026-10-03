package chute

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func SafeName(value string) string {
	return unsafeName.ReplaceAllString(value, "_")
}

func WriteBundle(bundle *Bundle, output string) error {
	absolute, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return err
	}

	if err := writeJSON(filepath.Join(absolute, "manifest.json"), map[string]any{
		"root":         bundle.Root,
		"files":        bundle.Inventory,
		"parse_errors": bundle.ParseErrors,
	}); err != nil {
		return err
	}

	counts := make(map[string]int)
	summaries := make([]map[string]any, 0, len(bundle.Resources))
	for _, resource := range bundle.Resources {
		counts[resource.Kind]++
		summaries = append(summaries, resource.Summary())
	}

	if err := writeJSON(filepath.Join(absolute, "index.json"), map[string]any{
		"resource_count": len(bundle.Resources),
		"counts_by_kind": counts,
		"resources":      summaries,
	}); err != nil {
		return err
	}

	volumesRoot := filepath.Join(absolute, "volumes")
	if err := os.MkdirAll(volumesRoot, 0o755); err != nil {
		return err
	}

	for _, volume := range bundle.Index.LonghornVolumes() {
		projection, err := ProjectVolume(bundle.Index, volume.Name)
		if err != nil {
			return err
		}
		logs := ExtractLogs(bundle.Root, bundle.Inventory, projection.Identifiers)
		if err := WriteProjection(projection, logs, filepath.Join(volumesRoot, SafeName(volume.Name))); err != nil {
			return err
		}
	}

	return nil
}

func WriteProjection(projection *VolumeProjection, logs LogEvidence, output string) error {
	absolute, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return err
	}

	exports := []struct {
		name      string
		resources []Resource
	}{
		{"volume.yaml", []Resource{projection.Volume}},
		{"engines.yaml", projection.Engines},
		{"replicas.yaml", projection.Replicas},
		{"pv.yaml", resourceSlice(projection.PV)},
		{"pvc.yaml", resourceSlice(projection.PVC)},
		{"pods.yaml", projection.Pods},
		{"volume_attachments.yaml", projection.Attachments},
		{"kubernetes_nodes.yaml", projection.KubernetesNodes},
		{"longhorn_nodes.yaml", projection.LonghornNodes},
	}

	for _, export := range exports {
		if err := writeYAML(filepath.Join(absolute, export.name), export.resources); err != nil {
			return err
		}
	}

	if err := writeJSON(filepath.Join(absolute, "evidence.json"), evidenceDocument(projection, logs)); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(absolute, "sources.json"), sourceDocument(projection)); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(absolute, "relevant_logs.log"), []byte(renderLogs(logs)), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(absolute, "summary.md"), []byte(renderSummary(projection, logs)), 0o644)
}

func resourceSlice(resource *Resource) []Resource {
	if resource == nil {
		return nil
	}
	return []Resource{*resource}
}

func evidenceDocument(projection *VolumeProjection, logs LogEvidence) map[string]any {
	return map[string]any{
		"volume": projection.Volume.Summary(),
		"observed": map[string]any{
			"state":      nullableString(nestedString(projection.Volume.Data, "status", "state")),
			"robustness": nullableString(nestedString(projection.Volume.Data, "status", "robustness")),
		},
		"related_resource_counts": map[string]int{
			"engines":           len(projection.Engines),
			"replicas":          len(projection.Replicas),
			"pods":              len(projection.Pods),
			"volumeAttachments": len(projection.Attachments),
			"kubernetesNodes":   len(projection.KubernetesNodes),
			"longhornNodes":     len(projection.LonghornNodes),
		},
		"identifiers_used_for_log_matching": projection.Identifiers,
		"log_evidence": map[string]any{
			"matched_lines":  logs.MatchedLines,
			"included_lines": logs.IncludedLines,
			"truncated":      logs.Truncated,
			"read_errors":    logs.Errors,
		},
	}
}

func sourceDocument(projection *VolumeProjection) []map[string]any {
	resources := []Resource{projection.Volume}
	resources = append(resources, resourceSlice(projection.PV)...)
	resources = append(resources, resourceSlice(projection.PVC)...)
	resources = append(resources, projection.Engines...)
	resources = append(resources, projection.Replicas...)
	resources = append(resources, projection.Pods...)
	resources = append(resources, projection.Attachments...)
	resources = append(resources, projection.KubernetesNodes...)
	resources = append(resources, projection.LonghornNodes...)

	seen := make(map[string]struct{})
	var sources []map[string]any
	for _, resource := range resources {
		key := resource.Kind + "\x00" + resource.Namespace + "\x00" + resource.Name + "\x00" + resource.Source
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		sources = append(sources, resource.SourceRef())
	}
	return sources
}

func renderLogs(logs LogEvidence) string {
	var builder strings.Builder
	for _, match := range logs.Matches {
		fmt.Fprintf(&builder, "[%s:%d] %s\n", match.Source, match.Line, match.Text)
	}
	if logs.Truncated {
		fmt.Fprintf(&builder, "\n# Truncated: %d matching lines found; %d included.\n", logs.MatchedLines, logs.IncludedLines)
	}
	return builder.String()
}

func renderSummary(projection *VolumeProjection, logs LogEvidence) string {
	return fmt.Sprintf(`# Volume %s

This file is a deterministic summary of observed bundle content. It is not a root-cause determination.

## Observed state

- State: %s
- Robustness: %s
- PersistentVolume: %s
- PersistentVolumeClaim: %s

## Related resources

- Engines: %d
- Replicas: %d
- Pods: %d
- VolumeAttachments: %d
- Kubernetes nodes: %d
- Longhorn nodes: %d

## Log evidence

- Matching identifiers: %d
- Matching lines found: %d
- Lines included: %d
- Truncated: %t

Every exported resource retains its original support-bundle source in sources.json.
`,
		projection.Volume.Name,
		displayString(nestedString(projection.Volume.Data, "status", "state")),
		displayString(nestedString(projection.Volume.Data, "status", "robustness")),
		resourceName(projection.PV),
		resourceName(projection.PVC),
		len(projection.Engines),
		len(projection.Replicas),
		len(projection.Pods),
		len(projection.Attachments),
		len(projection.KubernetesNodes),
		len(projection.LonghornNodes),
		len(projection.Identifiers),
		logs.MatchedLines,
		logs.IncludedLines,
		logs.Truncated,
	)
}

func resourceName(resource *Resource) string {
	if resource == nil {
		return "not observed"
	}
	return resource.Name
}

func displayString(value string) string {
	if value == "" {
		return "not observed"
	}
	return value
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func writeJSON(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return os.WriteFile(path, content, 0o644)
}

func writeYAML(path string, resources []Resource) error {
	if len(resources) == 0 {
		return os.WriteFile(path, []byte("# No matching resources found in bundle.\n"), 0o644)
	}

	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	for _, resource := range resources {
		if err := encoder.Encode(resource.Data); err != nil {
			encoder.Close()
			return err
		}
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buffer.Bytes(), 0o644)
}
