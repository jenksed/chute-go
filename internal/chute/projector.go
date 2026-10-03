package chute

import (
	"fmt"
	"strings"
)

type VolumeProjection struct {
	Volume          Resource
	PV              *Resource
	PVC             *Resource
	Pods            []Resource
	Attachments     []Resource
	Engines         []Resource
	Replicas        []Resource
	KubernetesNodes []Resource
	LonghornNodes   []Resource
	Identifiers     []string
}

func ProjectVolume(index *Index, volumeName string) (*VolumeProjection, error) {
	var volume *Resource
	for _, candidate := range index.LonghornVolumes() {
		if candidate.Name == volumeName {
			copy := candidate
			volume = &copy
			break
		}
	}
	if volume == nil {
		return nil, fmt.Errorf("Longhorn volume not found: %s", volumeName)
	}

	projection := &VolumeProjection{Volume: *volume}
	projection.PV = findPV(index, *volume)
	projection.PVC = findPVC(index, *volume, projection.PV)
	projection.Pods = findPods(index, projection.PVC)
	projection.Attachments = findAttachments(index, projection.PV)
	projection.Engines = relatedLonghornResources(index, "Engine", volumeName)
	projection.Replicas = relatedLonghornResources(index, "Replica", volumeName)

	nodeNames := uniqueNonEmpty(
		append(
			append(
				append(
					mapResourceStrings(projection.Engines, func(r Resource) string {
						return nestedString(r.Data, "spec", "nodeID")
					}),
					mapResourceStrings(projection.Replicas, func(r Resource) string {
						return nestedString(r.Data, "spec", "nodeID")
					})...,
				),
				mapResourceStrings(projection.Pods, func(r Resource) string {
					return nestedString(r.Data, "spec", "nodeName")
				})...,
			),
			mapResourceStrings(projection.Attachments, func(r Resource) string {
				return nestedString(r.Data, "spec", "nodeName")
			})...,
		),
	)

	for _, node := range index.Kind("Node") {
		if !contains(nodeNames, node.Name) {
			continue
		}
		if node.Longhorn() {
			projection.LonghornNodes = append(projection.LonghornNodes, node)
		} else {
			projection.KubernetesNodes = append(projection.KubernetesNodes, node)
		}
	}

	projection.Identifiers = projectionIdentifiers(projection)
	return projection, nil
}

func findPV(index *Index, volume Resource) *Resource {
	pvName := nestedString(volume.Data, "status", "kubernetesStatus", "pvName")

	for _, pv := range index.Kind("PersistentVolume") {
		if (pvName != "" && pv.Name == pvName) ||
			pv.Name == volume.Name ||
			nestedString(pv.Data, "spec", "csi", "volumeHandle") == volume.Name {
			copy := pv
			return &copy
		}
	}
	return nil
}

func findPVC(index *Index, volume Resource, pv *Resource) *Resource {
	namespace := ""
	name := ""

	if pv != nil {
		namespace = nestedString(pv.Data, "spec", "claimRef", "namespace")
		name = nestedString(pv.Data, "spec", "claimRef", "name")
	}
	if namespace == "" {
		namespace = nestedString(volume.Data, "status", "kubernetesStatus", "namespace")
	}
	if name == "" {
		name = nestedString(volume.Data, "status", "kubernetesStatus", "pvcName")
	}
	if name == "" {
		return nil
	}
	return index.Get("PersistentVolumeClaim", name, namespace)
}

func findPods(index *Index, pvc *Resource) []Resource {
	if pvc == nil {
		return nil
	}

	var pods []Resource
	for _, pod := range index.Kind("Pod") {
		if pod.Namespace != pvc.Namespace {
			continue
		}

		for _, volumeValue := range nestedSlice(pod.Data, "spec", "volumes") {
			volume, ok := volumeValue.(map[string]any)
			if !ok {
				continue
			}
			if nestedString(volume, "persistentVolumeClaim", "claimName") == pvc.Name {
				pods = append(pods, pod)
				break
			}
		}
	}
	return pods
}

func findAttachments(index *Index, pv *Resource) []Resource {
	if pv == nil {
		return nil
	}

	var attachments []Resource
	for _, attachment := range index.Kind("VolumeAttachment") {
		if nestedString(attachment.Data, "spec", "source", "persistentVolumeName") == pv.Name {
			attachments = append(attachments, attachment)
		}
	}
	return attachments
}

func relatedLonghornResources(index *Index, kind, volumeName string) []Resource {
	var resources []Resource
	for _, resource := range index.Longhorn(kind) {
		if nestedString(resource.Data, "spec", "volumeName") == volumeName ||
			nestedString(resource.Data, "metadata", "labels", "longhornvolume") == volumeName {
			resources = append(resources, resource)
		}
	}
	return resources
}

func projectionIdentifiers(projection *VolumeProjection) []string {
	var resources []Resource
	resources = append(resources, projection.Volume)

	if projection.PV != nil {
		resources = append(resources, *projection.PV)
	}
	if projection.PVC != nil {
		resources = append(resources, *projection.PVC)
	}
	resources = append(resources, projection.Engines...)
	resources = append(resources, projection.Replicas...)
	resources = append(resources, projection.Pods...)
	resources = append(resources, projection.Attachments...)

	var identifiers []string
	for _, resource := range resources {
		if len(resource.Name) >= 8 {
			identifiers = append(identifiers, resource.Name)
		}
		if len(resource.UID) >= 8 {
			identifiers = append(identifiers, resource.UID)
		}
	}
	return uniqueNonEmpty(identifiers)
}

func mapResourceStrings(resources []Resource, fn func(Resource) string) []string {
	out := make([]string, 0, len(resources))
	for _, resource := range resources {
		out = append(out, fn(resource))
	}
	return out
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
