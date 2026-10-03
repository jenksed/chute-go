# Chute output format

Chute output is designed to be inspectable by a human and straightforward to consume from code.

The generated files fall into four groups:

1. observed resource copies
2. selected log and event evidence
3. derived deterministic metadata
4. coverage and provenance

None of the generated summaries are a root-cause determination.

## Bundle-level output

A full `chute process` run writes:

```text
processed/
├── manifest.json
├── index.json
├── coverage.json
├── warnings.json
└── volumes/
    └── <volume>/
```

### manifest.json

Inventory of bundle artifacts Chute discovered.

Includes:

- logical source path
- byte size
- artifact category
- origin when the file came from a nested node archive
- associated node when applicable
- YAML parse errors
- nested node archive extraction status

Internal temporary filesystem paths are not exported as evidence paths.

### index.json

Machine-readable resource index summary.

Includes:

- total parsed resource count
- counts by Kubernetes/Longhorn kind
- normalized resource summaries
- source path for each parsed resource

This is useful for answering questions about the bundle as a whole without opening every YAML file.

### coverage.json

Processing coverage report.

Its purpose is to prevent this false inference:

> The command succeeded, therefore every artifact was understood.

Coverage includes counts for:

- top-level files
- node-archive files
- YAML files
- analyzed logs
- analyzed rotated logs
- node archives found
- node archives successfully extracted
- resources parsed
- unknown artifacts
- parse failures
- unsupported artifacts
- warnings

### warnings.json

Explicit partial-processing conditions.

Examples:

- YAML parse failure
- nested node archive extraction failure
- unknown artifacts that were inventoried but not interpreted

Warnings are evidence-quality metadata, not diagnoses.

## Volume case output

```text
case/
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

### summary.md

Human-readable deterministic summary.

Contains:

- observed Longhorn volume state
- observed robustness
- resolved PV and PVC
- counts of related resources
- counts of evidence identifiers by strength
- matching log counts by strength
- timeline entry count
- truncation state

The summary describes the package. It does not explain why the failure occurred.

### evidence.json

Machine-readable case metadata.

Contains:

- selected Longhorn volume identity and source
- observed state fields
- counts of related resources
- evidence identifiers and their tiers
- log evidence counts
- matched-line counts by tier
- context size
- truncation state
- read errors
- timeline entry count

Use this file when integrating Chute output with another deterministic tool or an AI harness.

### sources.json

Provenance list for exported resources.

Each entry identifies the resource and the support-bundle file it came from.

### timeline.md

Human-readable chronological view of timestamped evidence.

Entries come from:

- related Kubernetes Events
- log lines that independently matched an evidence identifier and contain a parseable RFC3339 timestamp

Context-only lines are not converted into timeline events.

Ordering is deterministic. Causation is not inferred.

### timeline.jsonl

One timeline entry per JSON line.

Useful for:

- downstream sorting/filtering
- scripts
- model input construction
- regression comparison

### relevant_logs.log

Bounded log evidence.

Each block records:

- source path
- source line range
- strongest evidence tier in the block
- identifiers that caused the block to be selected

Directly matching lines are marked with `*`.

Surrounding context lines are included without being treated as independent matches.

Example shape:

```text
[logs/.../csi-attacher.log.1:120-130] tier=primary matched=volume.name:pvc-abc123(primary)
  120: context
* 125: failed to attach pvc-abc123 to worker-1
  130: context
```

### events.yaml

Related Kubernetes Event resources copied from the bundle.

Relationship selection uses the same evidence identities used for log selection.

### Resource YAML files

The remaining YAML files are copies of observed related resources.

If a category has no matching resource, the file contains:

```text
# No matching resources found in bundle.
```

This makes absence explicit and preserves a stable case layout.

## Node case output

A node case contains the same summary/evidence/provenance/log/timeline concepts plus node-oriented resources:

```text
node-case/
├── summary.md
├── evidence.json
├── sources.json
├── timeline.md
├── timeline.jsonl
├── relevant_logs.log
├── events.yaml
├── kubernetes_nodes.yaml
├── longhorn_nodes.yaml
├── instance_managers.yaml
├── pods.yaml
├── engines.yaml
├── replicas.yaml
├── volume_attachments.yaml
├── volumes.yaml
└── node_bundle/
```

### node_bundle/

Files copied from the safely extracted nested node support archive.

Relative paths are preserved.

Chute currently treats these as source evidence. It does not pretend every arbitrary node artifact has a structured parser.

## Evidence tier semantics

### Primary

A direct storage identity:

- Longhorn Volume
- Longhorn Engine
- Longhorn Replica
- CSI VolumeAttachment

### Secondary

A Kubernetes storage identity bound to the Longhorn volume:

- PersistentVolume
- PersistentVolumeClaim

### Contextual

A neighboring operational identity:

- Pod
- Node
- other node/workload objects used by node projections

The tiers are not probability scores.

They answer:

> Why was this evidence selected, and how directly is the matching identity tied to the selected case?

## Provenance rule

Generated output must be traceable back to source evidence.

Chute preserves:

- resource source file
- log source file
- log source line numbers
- node archive logical path
- object identity

If a future feature cannot preserve that boundary, it should not silently become part of the deterministic evidence layer.
