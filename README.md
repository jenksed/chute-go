# Chute

Chute turns a Longhorn support bundle into smaller, deterministic evidence packages organized around the resources an engineer actually investigates.

Chute is an offline evidence transformer, not a Longhorn component and not a diagnostic authority.

## Current capability

Chute can:

- accept an extracted directory, `.zip`, `.tar.gz`, or `.tgz` support bundle
- inventory files and report processing coverage
- parse Kubernetes and Longhorn YAML resources
- recognize current, rotated, and gzip-compressed logs
- safely ingest nested `nodes/<node>.zip` evidence
- inspect discovered Longhorn volumes before choosing a case
- resolve a volume from a PVC or Pod selector
- connect PVCs, PVs, Pods, VolumeAttachments, Longhorn Volumes, Engines, Replicas, Nodes, Events, and InstanceManagers using explicit identifiers
- emit volume-oriented and node-oriented evidence cases
- label log matches as primary, secondary, or contextual evidence
- emit bounded, merged log-context windows
- emit deterministic timestamped evidence timelines
- record unknown and unsupported artifacts instead of silently ignoring them

It does not attempt root-cause diagnosis.

## Build

```bash
go build -o chute ./cmd/chute
```

## Usage

Inspect the volumes in a bundle:

```bash
./chute inspect supportbundle.zip
```

Process every Longhorn volume:

```bash
./chute process --output ./processed supportbundle.zip
```

Project one volume:

```bash
./chute volume --output ./case supportbundle.zip pvc-abc123
```

Resolve a volume from a PVC or Pod:

```bash
./chute volume --pvc default/data --output ./case supportbundle.zip
./chute volume --pod default/database-0 --output ./case supportbundle.zip
```

Project one node, including extracted node-bundle evidence when available:

```bash
./chute node --output ./node-case supportbundle.zip worker-1
```

Log evidence includes five lines before and after a matching line by default:

```bash
./chute volume --context 10 supportbundle.zip pvc-abc123
./chute node --context 10 supportbundle.zip worker-1
./chute process --context 0 supportbundle.zip
```

## Bundle output

```text
processed/
├── manifest.json
├── index.json
├── coverage.json
├── warnings.json
└── volumes/
    └── <volume>/
        ├── summary.md
        ├── evidence.json
        ├── sources.json
        ├── timeline.md
        ├── timeline.jsonl
        ├── relevant_logs.log
        ├── events.yaml
        ├── volume.yaml
        ├── engines.yaml
        ├── replicas.yaml
        ├── pv.yaml
        ├── pvc.yaml
        ├── pods.yaml
        ├── volume_attachments.yaml
        ├── kubernetes_nodes.yaml
        └── longhorn_nodes.yaml
```

A node case additionally contains related InstanceManagers and a `node_bundle/` directory containing safely extracted node-local evidence.

## Evidence tiers

Chute does not treat all string matches as equally strong.

```text
PRIMARY
  Longhorn Volume / Engine / Replica / CSI VolumeAttachment

SECONDARY
  PersistentVolume / PersistentVolumeClaim

CONTEXTUAL
  Pod / Node and other operational neighbors
```

Every evidence window records which identifiers matched and the strongest tier represented in that window.

Contextual evidence is retained because it can explain operational sequence. It must not be interpreted as proof that every object mentioned in the same line belongs to the selected volume.

## Coverage

`coverage.json` reports how much of the bundle Chute actually processed, including rotated logs and nested node archives.

`warnings.json` reports partial processing, parse failures, failed nested archive ingestion, and unknown artifacts.

Successful command execution therefore does not imply complete evidence coverage.

## Evidence boundary

1. Raw bundle artifacts remain authoritative.
2. Chute derives deterministic relationships from explicit resource identifiers.
3. Chute labels the strength of evidence used for log selection.
4. Timelines order observed evidence without inferring causation.
5. Root-cause judgment remains with the engineer or downstream analysis system.

## Archive handling

Archives are extracted into temporary directories and removed after the command completes.

The original bundle is never mutated. Archive path traversal and links are rejected. Nested node archives that cannot be safely processed are reported in warnings while the rest of the bundle remains available.

## Validation

The initial synthetic slice was followed by validation against a real Longhorn test support bundle. That run exposed three concrete gaps: rotated logs, nested node archives, and evidence-strength ambiguity around shared workload context.

The issue, impact, remediation, and acceptance rules are documented in [docs/REAL_BUNDLE_VALIDATION.md](docs/REAL_BUNDLE_VALIDATION.md).

Stable architectural decisions are recorded in [docs/DESIGN_DECISIONS.md](docs/DESIGN_DECISIONS.md).

The real support bundle is not committed. Regression tests reproduce the relevant structure with minimized fixture data.
