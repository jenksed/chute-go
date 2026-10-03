package chute

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

func WriteBundle(bundle *Bundle, output string, contextLines int) error {
	absolute, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return err
	}

	if err := writeJSON(filepath.Join(absolute, "manifest.json"), map[string]any{
		"input":         bundle.Input,
		"root":          bundle.Root,
		"files":         bundle.Inventory,
		"parse_errors":  bundle.ParseErrors,
		"node_archives": bundle.NodeArchives,
	}); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(absolute, "coverage.json"), BuildCoverage(bundle)); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(absolute, "warnings.json"), AllWarnings(bundle)); err != nil {
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
		logs := ExtractLogs(bundle.Root, bundle.Inventory, projection.EvidenceIdentifiers, contextLines)
		timeline := BuildTimeline(projection.Events, logs, projection.EvidenceIdentifiers)
		if err := WriteProjection(projection, logs, timeline, filepath.Join(volumesRoot, SafeName(volume.Name))); err != nil {
			return err
		}
	}

	return nil
}

func WriteProjection(projection *VolumeProjection, logs LogEvidence, timeline []TimelineEntry, output string) error {
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
		{"events.yaml", projection.Events},
	}

	for _, export := range exports {
		if err := writeYAML(filepath.Join(absolute, export.name), export.resources); err != nil {
			return err
		}
	}

	if err := writeJSON(filepath.Join(absolute, "evidence.json"), volumeEvidenceDocument(projection, logs, timeline)); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(absolute, "sources.json"), volumeSourceDocument(projection)); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(absolute, "relevant_logs.log"), []byte(renderLogs(logs)), 0o644); err != nil {
		return err
	}
	if err := writeTimeline(filepath.Join(absolute, "timeline.jsonl"), filepath.Join(absolute, "timeline.md"), timeline); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(absolute, "summary.md"), []byte(renderVolumeSummary(projection, logs, timeline)), 0o644)
}

func WriteNodeProjection(projection *NodeProjection, logs LogEvidence, timeline []TimelineEntry, output string) error {
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
		{"kubernetes_nodes.yaml", projection.KubernetesNodes},
		{"longhorn_nodes.yaml", projection.LonghornNodes},
		{"instance_managers.yaml", projection.InstanceManagers},
		{"pods.yaml", projection.Pods},
		{"engines.yaml", projection.Engines},
		{"replicas.yaml", projection.Replicas},
		{"volume_attachments.yaml", projection.Attachments},
		{"volumes.yaml", projection.Volumes},
		{"events.yaml", projection.Events},
	}
	for _, export := range exports {
		if err := writeYAML(filepath.Join(absolute, export.name), export.resources); err != nil {
			return err
		}
	}

	if err := copyNodeArtifacts(projection, filepath.Join(absolute, "node_bundle")); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(absolute, "evidence.json"), nodeEvidenceDocument(projection, logs, timeline)); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(absolute, "sources.json"), nodeSourceDocument(projection)); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(absolute, "relevant_logs.log"), []byte(renderLogs(logs)), 0o644); err != nil {
		return err
	}
	if err := writeTimeline(filepath.Join(absolute, "timeline.jsonl"), filepath.Join(absolute, "timeline.md"), timeline); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(absolute, "summary.md"), []byte(renderNodeSummary(projection, logs, timeline)), 0o644)
}

func resourceSlice(resource *Resource) []Resource {
	if resource == nil {
		return nil
	}
	return []Resource{*resource}
}

func volumeEvidenceDocument(projection *VolumeProjection, logs LogEvidence, timeline []TimelineEntry) map[string]any {
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
			"events":            len(projection.Events),
		},
		"evidence_identifiers": projection.EvidenceIdentifiers,
		"log_evidence": map[string]any{
			"matched_lines":   logs.MatchedLines,
			"matched_by_tier": logs.MatchedByTier,
			"included_lines":  logs.IncludedLines,
			"context_lines":   logs.ContextLines,
			"windows":         len(logs.Windows),
			"truncated":       logs.Truncated,
			"read_errors":     logs.Errors,
		},
		"timeline_entries": len(timeline),
	}
}

func nodeEvidenceDocument(projection *NodeProjection, logs LogEvidence, timeline []TimelineEntry) map[string]any {
	return map[string]any{
		"node": projection.NodeName,
		"related_resource_counts": map[string]int{
			"kubernetesNodes":   len(projection.KubernetesNodes),
			"longhornNodes":     len(projection.LonghornNodes),
			"instanceManagers":  len(projection.InstanceManagers),
			"pods":              len(projection.Pods),
			"engines":           len(projection.Engines),
			"replicas":          len(projection.Replicas),
			"volumeAttachments": len(projection.Attachments),
			"volumes":           len(projection.Volumes),
			"events":            len(projection.Events),
			"nodeArtifacts":     len(projection.Artifacts),
		},
		"evidence_identifiers": projection.EvidenceIdentifiers,
		"log_evidence": map[string]any{
			"matched_lines":   logs.MatchedLines,
			"matched_by_tier": logs.MatchedByTier,
			"included_lines":  logs.IncludedLines,
			"context_lines":   logs.ContextLines,
			"windows":         len(logs.Windows),
			"truncated":       logs.Truncated,
			"read_errors":     logs.Errors,
		},
		"timeline_entries": len(timeline),
	}
}

func volumeSourceDocument(projection *VolumeProjection) []map[string]any {
	resources := []Resource{projection.Volume}
	resources = append(resources, resourceSlice(projection.PV)...)
	resources = append(resources, resourceSlice(projection.PVC)...)
	resources = append(resources, projection.Engines...)
	resources = append(resources, projection.Replicas...)
	resources = append(resources, projection.Pods...)
	resources = append(resources, projection.Attachments...)
	resources = append(resources, projection.KubernetesNodes...)
	resources = append(resources, projection.LonghornNodes...)
	resources = append(resources, projection.Events...)
	return sourceRefs(resources)
}

func nodeSourceDocument(projection *NodeProjection) []map[string]any {
	var resources []Resource
	resources = append(resources, projection.KubernetesNodes...)
	resources = append(resources, projection.LonghornNodes...)
	resources = append(resources, projection.InstanceManagers...)
	resources = append(resources, projection.Pods...)
	resources = append(resources, projection.Engines...)
	resources = append(resources, projection.Replicas...)
	resources = append(resources, projection.Attachments...)
	resources = append(resources, projection.Volumes...)
	resources = append(resources, projection.Events...)

	sources := sourceRefs(resources)
	for _, artifact := range projection.Artifacts {
		sources = append(sources, map[string]any{
			"kind":   "node_artifact",
			"node":   projection.NodeName,
			"source": artifact.Path,
		})
	}
	return sources
}

func sourceRefs(resources []Resource) []map[string]any {
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
	for _, window := range logs.Windows {
		fmt.Fprintf(
			&builder,
			"[%s:%d-%d] tier=%s matched=%s\n",
			window.Source,
			window.StartLine,
			window.EndLine,
			window.Tier,
			renderMatchedIdentifiers(window.MatchedIdentifiers),
		)
		for _, line := range window.Lines {
			prefix := " "
			if len(line.Matches) > 0 {
				prefix = "*"
			}
			fmt.Fprintf(&builder, "%s %d: %s\n", prefix, line.Line, line.Text)
		}
		builder.WriteString("\n")
	}
	if logs.Truncated {
		fmt.Fprintf(&builder, "# Truncated after %d included log lines.\n", logs.IncludedLines)
	}
	return builder.String()
}

func renderMatchedIdentifiers(identifiers []EvidenceIdentifier) string {
	parts := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		parts = append(parts, fmt.Sprintf("%s:%s(%s)", identifier.Kind, identifier.Value, identifier.Tier))
	}
	return strings.Join(parts, ",")
}

func renderVolumeSummary(projection *VolumeProjection, logs LogEvidence, timeline []TimelineEntry) string {
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
- Related events: %d

## Evidence tiers

- Primary identifiers: %d
- Secondary identifiers: %d
- Contextual identifiers: %d

## Log evidence

- Matching lines found: %d
- Primary matches: %d
- Secondary matches: %d
- Contextual matches: %d
- Context lines requested: %d
- Evidence windows: %d
- Lines included: %d
- Truncated: %t

## Timeline

- Timestamped evidence entries: %d

Every exported resource retains its original support-bundle source in sources.json. Contextual evidence is intentionally labeled and must not be treated as equivalent to exact storage identity evidence.
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
		len(projection.Events),
		countIdentifiers(projection.EvidenceIdentifiers, TierPrimary),
		countIdentifiers(projection.EvidenceIdentifiers, TierSecondary),
		countIdentifiers(projection.EvidenceIdentifiers, TierContextual),
		logs.MatchedLines,
		logs.MatchedByTier[TierPrimary],
		logs.MatchedByTier[TierSecondary],
		logs.MatchedByTier[TierContextual],
		logs.ContextLines,
		len(logs.Windows),
		logs.IncludedLines,
		logs.Truncated,
		len(timeline),
	)
}

func renderNodeSummary(projection *NodeProjection, logs LogEvidence, timeline []TimelineEntry) string {
	return fmt.Sprintf(`# Node %s

This file is a deterministic node-oriented summary of observed bundle content. It is not a root-cause determination.

## Related resources

- Kubernetes node objects: %d
- Longhorn node objects: %d
- Instance managers: %d
- Pods: %d
- Engines: %d
- Replicas: %d
- VolumeAttachments: %d
- Volumes: %d
- Related events: %d
- Extracted node artifacts: %d

## Log evidence

- Matching lines found: %d
- Primary matches: %d
- Secondary matches: %d
- Contextual matches: %d
- Evidence windows: %d
- Lines included: %d
- Truncated: %t

## Timeline

- Timestamped evidence entries: %d

Extracted node-bundle artifacts are copied under node_bundle/ with their relative paths preserved.
`,
		projection.NodeName,
		len(projection.KubernetesNodes),
		len(projection.LonghornNodes),
		len(projection.InstanceManagers),
		len(projection.Pods),
		len(projection.Engines),
		len(projection.Replicas),
		len(projection.Attachments),
		len(projection.Volumes),
		len(projection.Events),
		len(projection.Artifacts),
		logs.MatchedLines,
		logs.MatchedByTier[TierPrimary],
		logs.MatchedByTier[TierSecondary],
		logs.MatchedByTier[TierContextual],
		len(logs.Windows),
		logs.IncludedLines,
		logs.Truncated,
		len(timeline),
	)
}

func countIdentifiers(identifiers []EvidenceIdentifier, tier EvidenceTier) int {
	count := 0
	for _, identifier := range identifiers {
		if identifier.Tier == tier {
			count++
		}
	}
	return count
}

func writeTimeline(jsonlPath, markdownPath string, timeline []TimelineEntry) error {
	file, err := os.Create(jsonlPath)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	for _, entry := range timeline {
		if err := encoder.Encode(entry); err != nil {
			file.Close()
			return err
		}
	}
	if err := writer.Flush(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	var markdown strings.Builder
	markdown.WriteString("# Evidence timeline\n\n")
	markdown.WriteString("This timeline orders timestamped observed events and matching log records. It does not infer causation.\n\n")
	for _, entry := range timeline {
		fmt.Fprintf(
			&markdown,
			"- %s [%s] [%s] %s",
			entry.Timestamp,
			entry.Tier,
			entry.Type,
			entry.Summary,
		)
		if entry.Line > 0 {
			fmt.Fprintf(&markdown, " (%s:%d)", entry.Source, entry.Line)
		} else {
			fmt.Fprintf(&markdown, " (%s)", entry.Source)
		}
		markdown.WriteString("\n")
	}
	return os.WriteFile(markdownPath, []byte(markdown.String()), 0o644)
}

func copyNodeArtifacts(projection *NodeProjection, output string) error {
	if len(projection.Artifacts) == 0 {
		return os.MkdirAll(output, 0o755)
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}

	prefix := filepath.ToSlash(filepath.Join("nodes", projection.NodeName, "bundle")) + "/"
	for _, artifact := range projection.Artifacts {
		relative := strings.TrimPrefix(artifact.Path, prefix)
		if relative == artifact.Path || relative == "" {
			continue
		}
		target, err := safeArchivePath(output, relative)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := copyFile(physicalPath("", artifact), target); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
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
