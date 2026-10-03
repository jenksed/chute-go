package chute

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectVolumeBuildsRelatedEvidence(t *testing.T) {
	root := fixtureBundle(t)

	bundle, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(bundle.ParseErrors) != 0 {
		t.Fatalf("unexpected parse errors: %#v", bundle.ParseErrors)
	}

	projection, err := ProjectVolume(bundle.Index, "pvc-abc123")
	if err != nil {
		t.Fatalf("ProjectVolume: %v", err)
	}

	if projection.Volume.Name != "pvc-abc123" {
		t.Fatalf("volume = %q", projection.Volume.Name)
	}
	if projection.PV == nil || projection.PV.Name != "pvc-abc123" {
		t.Fatalf("unexpected PV: %#v", projection.PV)
	}
	if projection.PVC == nil || projection.PVC.Name != "data" {
		t.Fatalf("unexpected PVC: %#v", projection.PVC)
	}

	assertNames(t, projection.Engines, []string{"pvc-abc123-e-0"})
	assertNames(t, projection.Replicas, []string{"pvc-abc123-r-1", "pvc-abc123-r-2"})
	assertNames(t, projection.Pods, []string{"database-0"})
	assertNames(t, projection.Attachments, []string{"csi-attachment-abc123"})
	assertNames(t, projection.KubernetesNodes, []string{"worker-1"})
	assertNames(t, projection.LonghornNodes, []string{"worker-1"})
}

func TestWriteProjectionProducesBoundedCaseWithProvenance(t *testing.T) {
	root := fixtureBundle(t)
	bundle, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	projection, err := ProjectVolume(bundle.Index, "pvc-abc123")
	if err != nil {
		t.Fatalf("ProjectVolume: %v", err)
	}
	logs := ExtractLogs(bundle.Root, bundle.Inventory, projection.Identifiers)

	output := t.TempDir()
	if err := WriteProjection(projection, logs, output); err != nil {
		t.Fatalf("WriteProjection: %v", err)
	}

	for _, name := range []string{
		"summary.md",
		"evidence.json",
		"sources.json",
		"replicas.yaml",
		"relevant_logs.log",
	} {
		if _, err := os.Stat(filepath.Join(output, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}

	relevantLogs, err := os.ReadFile(filepath.Join(output, "relevant_logs.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(relevantLogs), "pvc-abc123") {
		t.Fatalf("expected volume evidence in relevant logs: %s", relevantLogs)
	}
	if strings.Contains(string(relevantLogs), "completely unrelated") {
		t.Fatalf("unrelated line leaked into relevant logs: %s", relevantLogs)
	}

	sources, err := os.ReadFile(filepath.Join(output, "sources.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sources), "yamls/resources.yaml") {
		t.Fatalf("expected source provenance: %s", sources)
	}
}

func TestWriteBundleProducesManifestIndexAndVolumeProjection(t *testing.T) {
	root := fixtureBundle(t)
	bundle, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	output := t.TempDir()
	if err := WriteBundle(bundle, output); err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}

	for _, path := range []string{
		"manifest.json",
		"index.json",
		filepath.Join("volumes", "pvc-abc123", "summary.md"),
	} {
		if _, err := os.Stat(filepath.Join(output, path)); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}
}

func TestProjectVolumeMissingVolumeFailsClearly(t *testing.T) {
	root := fixtureBundle(t)
	bundle, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	_, err = ProjectVolume(bundle.Index, "pvc-does-not-exist")
	if err == nil || !strings.Contains(err.Error(), "Longhorn volume not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertNames(t *testing.T, resources []Resource, expected []string) {
	t.Helper()

	if len(resources) != len(expected) {
		t.Fatalf("got %d resources, want %d: %#v", len(resources), len(expected), resources)
	}
	for i, resource := range resources {
		if resource.Name != expected[i] {
			t.Fatalf("resource %d = %q, want %q", i, resource.Name, expected[i])
		}
	}
}

func fixtureBundle(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "yamls"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "yamls", "resources.yaml"), []byte(fixtureYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "logs", "manager.log"), []byte(fixtureLog), 0o644); err != nil {
		t.Fatal(err)
	}

	return root
}

const fixtureLog = `time=2026-10-03T12:00:00Z level=error msg="failed to attach volume pvc-abc123"
time=2026-10-03T12:00:01Z level=info msg="completely unrelated event"
`

const fixtureYAML = `apiVersion: v1
kind: List
items:
  - apiVersion: longhorn.io/v1beta2
    kind: Volume
    metadata:
      name: pvc-abc123
      uid: volume-uid-abc123
    status:
      state: detached
      robustness: degraded
      kubernetesStatus:
        pvName: pvc-abc123
        namespace: default
        pvcName: data
  - apiVersion: longhorn.io/v1beta2
    kind: Engine
    metadata:
      name: pvc-abc123-e-0
      namespace: longhorn-system
    spec:
      volumeName: pvc-abc123
      nodeID: worker-1
  - apiVersion: longhorn.io/v1beta2
    kind: Replica
    metadata:
      name: pvc-abc123-r-1
      namespace: longhorn-system
    spec:
      volumeName: pvc-abc123
      nodeID: worker-1
  - apiVersion: longhorn.io/v1beta2
    kind: Replica
    metadata:
      name: pvc-abc123-r-2
      namespace: longhorn-system
    spec:
      volumeName: pvc-abc123
      nodeID: worker-2
  - apiVersion: v1
    kind: PersistentVolume
    metadata:
      name: pvc-abc123
    spec:
      claimRef:
        namespace: default
        name: data
      csi:
        driver: driver.longhorn.io
        volumeHandle: pvc-abc123
  - apiVersion: v1
    kind: PersistentVolumeClaim
    metadata:
      namespace: default
      name: data
    spec:
      volumeName: pvc-abc123
  - apiVersion: v1
    kind: Pod
    metadata:
      namespace: default
      name: database-0
    spec:
      nodeName: worker-1
      volumes:
        - name: storage
          persistentVolumeClaim:
            claimName: data
  - apiVersion: storage.k8s.io/v1
    kind: VolumeAttachment
    metadata:
      name: csi-attachment-abc123
    spec:
      nodeName: worker-1
      source:
        persistentVolumeName: pvc-abc123
  - apiVersion: v1
    kind: Node
    metadata:
      name: worker-1
  - apiVersion: longhorn.io/v1beta2
    kind: Node
    metadata:
      namespace: longhorn-system
      name: worker-1
`
