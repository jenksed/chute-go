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

func TestClassifyRotatedLogsAndNodeArchives(t *testing.T) {
	cases := map[string]string{
		"logs/longhorn-system/manager/manager.log":      "log",
		"logs/longhorn-system/manager/manager.log.1":    "log",
		"logs/longhorn-system/manager/manager.log.2.gz": "log",
		"nodes/worker-1.zip":                             "node_archive",
	}
	for path, expected := range cases {
		if got := classifyPath(path); got != expected {
			t.Fatalf("classifyPath(%q) = %q, want %q", path, got, expected)
		}
	}
}

func TestProjectVolumeBuildsRelatedEvidenceAndTiers(t *testing.T) {
	bundle, err := Load(fixtureBundle(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	projection, err := ProjectVolume(bundle.Index, "pvc-abc123")
	if err != nil {
		t.Fatalf("ProjectVolume: %v", err)
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

	assertIdentifierTier(t, projection.EvidenceIdentifiers, "pvc-abc123", TierPrimary)
	assertIdentifierTier(t, projection.EvidenceIdentifiers, "data", TierSecondary)
	assertIdentifierTier(t, projection.EvidenceIdentifiers, "database-0", TierContextual)
	assertIdentifierTier(t, projection.EvidenceIdentifiers, "worker-1", TierContextual)
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

func TestLoadInputIngestsNestedNodeArchive(t *testing.T) {
	root := fixtureBundle(t)
	bundle, cleanup, err := LoadInput(root)
	if err != nil {
		t.Fatalf("LoadInput: %v", err)
	}
	defer cleanup()

	if len(bundle.NodeArchives) != 1 {
		t.Fatalf("node archives = %#v", bundle.NodeArchives)
	}
	if bundle.NodeArchives[0].Node != "worker-1" || bundle.NodeArchives[0].ExtractedFiles == 0 {
		t.Fatalf("unexpected node archive status: %#v", bundle.NodeArchives[0])
	}

	found := false
	for _, entry := range bundle.Inventory {
		if entry.Node == "worker-1" && strings.HasSuffix(entry.Path, "logs/kubelet.log") {
			found = true
			if entry.Category != "log" || entry.Origin != "node_archive" {
				t.Fatalf("unexpected nested entry: %#v", entry)
			}
		}
	}
	if !found {
		t.Fatal("nested kubelet log not inventoried")
	}
}

func TestExtractLogsRanksCrossVolumeWorkloadContext(t *testing.T) {
	bundle, cleanup, err := LoadInput(fixtureBundle(t))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	projection, err := ProjectVolume(bundle.Index, "pvc-abc123")
	if err != nil {
		t.Fatal(err)
	}
	logs := ExtractLogs(bundle.Root, bundle.Inventory, projection.EvidenceIdentifiers, 0)

	if logs.MatchedByTier[TierPrimary] == 0 {
		t.Fatalf("expected primary matches: %#v", logs.MatchedByTier)
	}
	if logs.MatchedByTier[TierContextual] == 0 {
		t.Fatalf("expected contextual matches: %#v", logs.MatchedByTier)
	}

	var sawContextualOldVolume bool
	var sawRotated bool
	for _, window := range logs.Windows {
		if strings.HasSuffix(window.Source, ".log.1") {
			sawRotated = true
		}
		for _, line := range window.Lines {
			if strings.Contains(line.Text, "pvc-old999") && strings.Contains(line.Text, "database-0") {
				sawContextualOldVolume = true
				if strongestIdentifierTier(line.Matches) != TierContextual {
					t.Fatalf("cross-volume workload context was not contextual: %#v", line.Matches)
				}
			}
		}
	}
	if !sawContextualOldVolume {
		t.Fatal("expected shared-workload cross-volume context")
	}
	if !sawRotated {
		t.Fatal("rotated log was not analyzed")
	}
}

func TestProjectNodeIncludesInstanceManagerAndArtifacts(t *testing.T) {
	bundle, cleanup, err := LoadInput(fixtureBundle(t))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	projection, err := ProjectNode(bundle.Index, bundle.Inventory, "worker-1")
	if err != nil {
		t.Fatalf("ProjectNode: %v", err)
	}

	assertNames(t, projection.InstanceManagers, []string{"instance-manager-worker1"})
	if len(projection.Artifacts) == 0 {
		t.Fatal("expected node archive artifacts")
	}
	assertIdentifierTier(t, projection.EvidenceIdentifiers, "worker-1", TierPrimary)
}

func TestTimelineOrdersRelatedEventsAndLogs(t *testing.T) {
	bundle, cleanup, err := LoadInput(fixtureBundle(t))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	projection, err := ProjectVolume(bundle.Index, "pvc-abc123")
	if err != nil {
		t.Fatal(err)
	}
	logs := ExtractLogs(bundle.Root, bundle.Inventory, projection.EvidenceIdentifiers, 0)
	timeline := BuildTimeline(projection.Events, logs, projection.EvidenceIdentifiers)

	if len(timeline) < 3 {
		t.Fatalf("timeline too small: %#v", timeline)
	}
	for i := 1; i < len(timeline); i++ {
		if timeline[i].Timestamp < timeline[i-1].Timestamp {
			t.Fatalf("timeline not sorted: %#v", timeline)
		}
	}

	var sawEvent, sawLog bool
	for _, entry := range timeline {
		if entry.Type == "kubernetes_event" {
			sawEvent = true
		}
		if entry.Type == "log" {
			sawLog = true
		}
	}
	if !sawEvent || !sawLog {
		t.Fatalf("timeline missing source types: %#v", timeline)
	}
}

func TestCoverageReportsRealBundleGapsAsHandled(t *testing.T) {
	bundle, cleanup, err := LoadInput(fixtureBundle(t))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	coverage := BuildCoverage(bundle)
	if coverage.RotatedLogsAnalyzed == 0 {
		t.Fatalf("rotated logs = %d", coverage.RotatedLogsAnalyzed)
	}
	if coverage.NodeArchivesFound != 1 || coverage.NodeArchivesExtracted != 1 {
		t.Fatalf("node archive coverage = %#v", coverage)
	}
	if coverage.NodeArchiveFiles == 0 {
		t.Fatalf("node archive files = %d", coverage.NodeArchiveFiles)
	}
}

func TestWriteBundleAndNodeCaseProduceNewArtifacts(t *testing.T) {
	bundle, cleanup, err := LoadInput(fixtureBundle(t))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	output := t.TempDir()
	if err := WriteBundle(bundle, output, DefaultLogContext); err != nil {
		t.Fatalf("WriteBundle: %v", err)
	}
	for _, path := range []string{
		"manifest.json",
		"coverage.json",
		"warnings.json",
		"index.json",
		filepath.Join("volumes", "pvc-abc123", "timeline.md"),
		filepath.Join("volumes", "pvc-abc123", "timeline.jsonl"),
		filepath.Join("volumes", "pvc-abc123", "events.yaml"),
	} {
		if _, err := os.Stat(filepath.Join(output, path)); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}

	nodeProjection, err := ProjectNode(bundle.Index, bundle.Inventory, "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	nodeLogs := ExtractLogs(bundle.Root, bundle.Inventory, nodeProjection.EvidenceIdentifiers, 1)
	nodeTimeline := BuildTimeline(nodeProjection.Events, nodeLogs, nodeProjection.EvidenceIdentifiers)
	nodeOutput := filepath.Join(t.TempDir(), "node")
	if err := WriteNodeProjection(nodeProjection, nodeLogs, nodeTimeline, nodeOutput); err != nil {
		t.Fatalf("WriteNodeProjection: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nodeOutput, "node_bundle", "logs", "kubelet.log")); err != nil {
		t.Fatalf("nested node evidence not copied: %v", err)
	}
}

func TestInspectVolumesProducesUsefulRows(t *testing.T) {
	bundle, err := Load(fixtureBundle(t))
	if err != nil {
		t.Fatal(err)
	}

	rows := InspectVolumes(bundle.Index)
	if len(rows) != 2 {
		t.Fatalf("rows = %#v", rows)
	}

	var output bytes.Buffer
	if err := WriteInspection(&output, rows); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "pvc-abc123") || !strings.Contains(output.String(), "degraded") {
		t.Fatalf("inspection output = %q", output.String())
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

			if len(bundle.Index.LonghornVolumes()) != 2 {
				t.Fatalf("volumes = %#v", bundle.Index.LonghornVolumes())
			}
			if bundle.Input != archive {
				t.Fatalf("input = %q, want %q", bundle.Input, archive)
			}
			if len(bundle.NodeArchives) != 1 || bundle.NodeArchives[0].Error != "" {
				t.Fatalf("nested node archive not ingested: %#v", bundle.NodeArchives)
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
	bundle, err := Load(fixtureBundle(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	_, err = ProjectVolume(bundle.Index, "pvc-does-not-exist")
	if err == nil || !strings.Contains(err.Error(), "Longhorn volume not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func assertIdentifierTier(t *testing.T, identifiers []EvidenceIdentifier, value string, tier EvidenceTier) {
	t.Helper()
	for _, identifier := range identifiers {
		if identifier.Value == value {
			if identifier.Tier != tier {
				t.Fatalf("identifier %q tier = %q, want %q", value, identifier.Tier, tier)
			}
			return
		}
	}
	t.Fatalf("identifier %q not found in %#v", value, identifiers)
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
	for _, path := range []string{
		filepath.Join(root, "yamls"),
		filepath.Join(root, "logs", "longhorn-system", "manager"),
		filepath.Join(root, "nodes"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(root, "yamls", "resources.yaml"), []byte(fixtureYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "logs", "longhorn-system", "manager", "manager.log"), []byte("2026-01-05T04:10:00Z unrelated current log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "logs", "longhorn-system", "manager", "manager.log.1"), []byte(fixtureRotatedLog), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := createNodeArchive(filepath.Join(root, "nodes", "worker-1.zip")); err != nil {
		t.Fatal(err)
	}

	return root
}

func createNodeArchive(destination string) error {
	file, err := os.Create(destination)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)

	entry, err := writer.Create("logs/kubelet.log")
	if err != nil {
		return err
	}
	if _, err := entry.Write([]byte("2026-01-05T04:03:00Z worker-1 kubelet recovered\n")); err != nil {
		return err
	}

	entry, err = writer.Create("system/mounts.txt")
	if err != nil {
		return err
	}
	if _, err := entry.Write([]byte("/dev/longhorn mounted\n")); err != nil {
		return err
	}

	if err := writer.Close(); err != nil {
		return err
	}
	return file.Close()
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

const fixtureRotatedLog = `2026-01-05T03:57:38Z removed finalizer from previous volume pvc-old999
2026-01-05T03:57:50Z pod database-0 still references previous volume pvc-old999
2026-01-05T03:57:55Z failed to attach volume pvc-abc123 to worker-1: node worker-1 is not ready
2026-01-05T04:02:32Z instance-manager-worker1 is not running on worker-1
2026-01-05T04:27:56Z worker-1 reported down
`

const fixtureYAML = `apiVersion: v1
kind: List
items:
  - apiVersion: longhorn.io/v1beta2
    kind: Volume
    metadata:
      name: pvc-abc123
      uid: volume-uid-abc123
      namespace: longhorn-system
    status:
      state: detached
      robustness: degraded
      currentNodeID: worker-1
      kubernetesStatus:
        pvName: pvc-abc123
        namespace: default
        pvcName: data
  - apiVersion: longhorn.io/v1beta2
    kind: Volume
    metadata:
      name: pvc-old999
      uid: volume-uid-old999
      namespace: longhorn-system
    status:
      state: detached
      robustness: unknown
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
  - apiVersion: longhorn.io/v1beta2
    kind: InstanceManager
    metadata:
      name: instance-manager-worker1
      namespace: longhorn-system
    spec:
      nodeID: worker-1
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
      uid: pvc-uid-data
    spec:
      volumeName: pvc-abc123
  - apiVersion: v1
    kind: Pod
    metadata:
      namespace: default
      name: database-0
      uid: pod-uid-database0
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
      uid: attachment-uid-abc123
    spec:
      nodeName: worker-1
      source:
        persistentVolumeName: pvc-abc123
  - apiVersion: v1
    kind: Node
    metadata:
      name: worker-1
      uid: kube-node-uid-worker1
  - apiVersion: longhorn.io/v1beta2
    kind: Node
    metadata:
      namespace: longhorn-system
      name: worker-1
      uid: longhorn-node-uid-worker1
  - apiVersion: v1
    kind: Event
    metadata:
      namespace: longhorn-system
      name: worker-1-down
      uid: event-worker1-down
      creationTimestamp: "2026-01-05T04:27:56Z"
    involvedObject:
      kind: Node
      name: worker-1
      uid: longhorn-node-uid-worker1
    type: Warning
    reason: Ready
    message: Node worker-1 is down
  - apiVersion: events.k8s.io/v1
    kind: Event
    metadata:
      namespace: longhorn-system
      name: pvc-abc123-attach
      uid: event-volume-attach
      creationTimestamp: "2026-01-05T03:57:55Z"
    regarding:
      kind: Volume
      name: pvc-abc123
      uid: volume-uid-abc123
    type: Warning
    reason: FailedAttach
    note: Failed to attach pvc-abc123 to worker-1
`
