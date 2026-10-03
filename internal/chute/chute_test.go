package chute

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
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

func TestResolveVolumeByPVCAndPod(t *testing.T) {
	bundle, err := Load(fixtureBundle(t))
	if err != nil {
		t.Fatal(err)
	}

	volume, err := ResolveVolumeByPVC(bundle.Index, "default/data")
	if err != nil {
		t.Fatalf("ResolveVolumeByPVC: %v", err)
	}
	if volume != "pvc-abc123" {
		t.Fatalf("volume = %q", volume)
	}

	volume, err = ResolveVolumeByPod(bundle.Index, "default/database-0")
	if err != nil {
		t.Fatalf("ResolveVolumeByPod: %v", err)
	}
	if volume != "pvc-abc123" {
		t.Fatalf("volume = %q", volume)
	}
}

func TestInspectVolumesProducesUsefulRows(t *testing.T) {
	bundle, err := Load(fixtureBundle(t))
	if err != nil {
		t.Fatal(err)
	}

	rows := InspectVolumes(bundle.Index)
	if len(rows) != 1 {
		t.Fatalf("rows = %#v", rows)
	}
	if rows[0].PVC != "data" || rows[0].Namespace != "default" || rows[0].Replicas != 2 {
		t.Fatalf("unexpected row: %#v", rows[0])
	}

	var output bytes.Buffer
	if err := WriteInspection(&output, rows); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "pvc-abc123") || !strings.Contains(output.String(), "degraded") {
		t.Fatalf("inspection output = %q", output.String())
	}
}

func TestExtractLogsIncludesAndMergesBoundedContext(t *testing.T) {
	root := fixtureBundle(t)
	bundle, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := ProjectVolume(bundle.Index, "pvc-abc123")
	if err != nil {
		t.Fatal(err)
	}

	logs := ExtractLogs(bundle.Root, bundle.Inventory, projection.Identifiers, 2)
	if logs.MatchedLines != 2 {
		t.Fatalf("matched lines = %d", logs.MatchedLines)
	}
	if len(logs.Windows) != 1 {
		t.Fatalf("windows = %#v", logs.Windows)
	}

	window := logs.Windows[0]
	if window.StartLine != 2 || window.EndLine != 8 {
		t.Fatalf("window range = %d-%d", window.StartLine, window.EndLine)
	}
	if len(window.Lines) != 7 {
		t.Fatalf("window lines = %d", len(window.Lines))
	}
	if !contains(window.MatchedIdentifiers, "pvc-abc123") {
		t.Fatalf("missing matched identifier: %#v", window.MatchedIdentifiers)
	}
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
	logs := ExtractLogs(bundle.Root, bundle.Inventory, projection.Identifiers, 1)

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
	if strings.Contains(string(relevantLogs), "far-away unrelated event") {
		t.Fatalf("far-away unrelated line leaked into relevant logs: %s", relevantLogs)
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
	if err := WriteBundle(bundle, output, DefaultLogContext); err != nil {
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

func TestLoadInputAcceptsZipAndTarGz(t *testing.T) {
	for _, kind := range []string{"zip", "tar.gz"} {
		t.Run(kind, func(t *testing.T) {
			root := fixtureBundle(t)
			archive := filepath.Join(t.TempDir(), "supportbundle."+kind)

			var err error
			if kind == "zip" {
				err = createZipFixture(root, archive)
			} else {
				err = createTarGzFixture(root, archive)
			}
			if err != nil {
				t.Fatal(err)
			}

			bundle, cleanup, err := LoadInput(archive)
			if err != nil {
				t.Fatalf("LoadInput: %v", err)
			}
			defer cleanup()

			if len(bundle.Index.LonghornVolumes()) != 1 {
				t.Fatalf("volumes = %#v", bundle.Index.LonghornVolumes())
			}
			if bundle.Input != archive {
				t.Fatalf("input = %q, want %q", bundle.Input, archive)
			}
		})
	}
}

func TestArchiveExtractionRejectsPathTraversal(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "bad.zip")
	writer, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(writer)
	entry, err := zipWriter.Create("../escape")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("bad")); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	_, cleanup, err := LoadInput(archive)
	cleanup()
	if err == nil || !strings.Contains(err.Error(), "unsafe archive path") {
		t.Fatalf("unexpected error: %v", err)
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

func createZipFixture(root, destination string) error {
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)

	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		entry, err := writer.Create(filepath.ToSlash(filepath.Join("bundle", relative)))
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = entry.Write(data)
		return err
	})
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func createTarGzFixture(root, destination string) error {
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	gzipWriter := gzip.NewWriter(file)
	writer := tar.NewWriter(gzipWriter)

	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		header := &tar.Header{
			Name: filepath.ToSlash(filepath.Join("bundle", relative)),
			Mode: 0o644,
			Size: int64(len(data)),
		}
		if err := writer.WriteHeader(header); err != nil {
			return err
		}
		_, err = writer.Write(data)
		return err
	})
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if closeErr := gzipWriter.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

const fixtureLog = `line 1 unrelated
line 2 context before
line 3 context before
line 4 failed to attach volume pvc-abc123
line 5 between related failures
line 6 replica pvc-abc123-r-1 stopped
line 7 context after
line 8 context after
line 9 unrelated
line 10 unrelated
line 11 unrelated
line 12 far-away unrelated event
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
