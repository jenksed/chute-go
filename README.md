# Chute

Chute turns a Longhorn support bundle into a smaller, deterministic evidence package organized around the resources an engineer actually investigates.

Chute is an offline evidence transformer, not a Longhorn component and not a diagnostic authority.

## Current capability

Chute can:

- accept an extracted directory, `.zip`, `.tar.gz`, or `.tgz` support bundle
- inventory files and classify likely YAML, logs, node data, and unknown artifacts
- parse Kubernetes and Longhorn YAML resources
- index resources by kind and name
- inspect discovered Longhorn volumes before choosing a case
- resolve a volume from a PVC or Pod selector
- connect PVCs, PVs, Pods, VolumeAttachments, Longhorn volumes, engines, replicas, and nodes using explicit identifiers
- emit a per-volume case directory containing related objects and provenance
- extract bounded, deduplicated log-context windows around matching resource identifiers
- record unclassified files in the manifest without copying the original bundle

It does not attempt root-cause diagnosis.

## Build

```bash
go build -o chute ./cmd/chute
```

The result is a single executable with no Go runtime installation required on the target machine.

## Usage

Inspect the volumes in a bundle before selecting a case:

```bash
./chute inspect supportbundle.zip
```

Example output:

```text
VOLUME       PVC   NAMESPACE  STATE     ROBUSTNESS  REPLICAS  PODS
pvc-abc123   data  default    detached  degraded    2         1
```

Process every Longhorn volume found in a bundle:

```bash
./chute process --output ./processed supportbundle.zip
```

Project one volume directly:

```bash
./chute volume --output ./case supportbundle.zip pvc-abc123
```

Resolve the volume from a PVC or Pod:

```bash
./chute volume --pvc default/data --output ./case supportbundle.zip
./chute volume --pod default/database-0 --output ./case supportbundle.zip
```

Log evidence includes five lines before and after a matching line by default. Change it per command:

```bash
./chute volume --context 10 supportbundle.zip pvc-abc123
./chute process --context 0 supportbundle.zip
```

Overlapping context windows are merged so the same evidence is not repeated.

## Output

```text
processed/
├── manifest.json
├── index.json
└── volumes/
    └── <volume>/
        ├── summary.md
        ├── evidence.json
        ├── sources.json
        ├── volume.yaml
        ├── engines.yaml
        ├── replicas.yaml
        ├── pv.yaml
        ├── pvc.yaml
        ├── pods.yaml
        ├── volume_attachments.yaml
        ├── kubernetes_nodes.yaml
        ├── longhorn_nodes.yaml
        └── relevant_logs.log
```

Each log evidence block records its source path, line range, and the identifiers that caused the match.

## Evidence boundary

Chute separates four concerns:

1. Raw bundle artifacts remain the source evidence.
2. Chute derives deterministic relationships from explicit resource identifiers.
3. Exported summaries describe observed state and related evidence.
4. Root-cause judgment remains with the engineer or downstream analysis system.

Correlation is not emitted as causation.

## Archive handling

Archives are extracted into a temporary directory and removed after the command completes. Archive entries that attempt path traversal or use symlinks/hard links are rejected.

## Current acceptance boundary

Automated tests cover YAML parsing, Longhorn/Kubernetes relationship projection, PVC/Pod lookup, inspection output, ZIP and tar.gz ingestion, archive path-traversal rejection, provenance export, bounded/merged log windows, and case generation.

The next meaningful acceptance step remains a real Longhorn support bundle. Real bundle structure and version differences should drive compatibility changes rather than speculative format support.
