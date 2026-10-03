# Real-bundle validation: January 5, 2026 Longhorn test bundle

This document records the first validation of Chute against a real Longhorn support bundle and the changes that validation drove.

The source bundle used for validation was kept local and is not committed to this repository. The regression tests added after this validation reproduce the relevant structure with minimized synthetic fixture data.

## What worked

Chute successfully parsed the real bundle and built a useful volume-oriented case.

For the affected volume, the first implementation correctly connected:

- Longhorn Volume
- Kubernetes PersistentVolume and PersistentVolumeClaim
- workload Pod
- CSI VolumeAttachment
- Longhorn Engine
- Longhorn Replicas
- Kubernetes Nodes
- Longhorn Nodes
- identifier-matching log evidence

The resulting evidence package exposed repeated CSI attach failures to a node Longhorn reported as not ready. Later evidence showed the node transitioning through Ready/down states and InstanceManager readiness failures. This validated the core resource-graph approach.

## Issues discovered

### 1. Rotated logs were silently omitted

The real bundle contained files such as:

```text
logs/.../longhorn-manager.log.1
logs/.../csi-attacher.log.1
```

The original classifier only recognized a final `.log` extension and checked for `/logs/` inside a root-relative path. A path beginning with `logs/` therefore did not satisfy the directory check, and `.log.1` did not have a `.log` extension.

Impact:

- valid historical evidence could be excluded from every case
- the operator had no direct indication that this evidence was skipped
- failures occurring shortly before log rotation could be missed entirely

Fix:

- normalize logical paths before directory classification
- recognize `.log`, `.log.N`, `.log.gz`, and `.log.N.gz`
- read gzip-compressed logs transparently
- report rotated-log coverage in `coverage.json`
- add a regression test specifically for a root-relative `logs/.../*.log.1`

### 2. Nested node support bundles were opaque

The real Longhorn support bundle contained one ZIP archive per node under `nodes/`. The node involved in the attach failure had its own nested archive, but Chute inventoried that ZIP as unknown and did not inspect its contents.

Impact:

- Chute could establish that an attach failed because the node was not ready
- it could not expose the node-local evidence needed to investigate why
- kubelet, mount, disk, kernel, and service evidence could remain hidden

Fix:

- classify `nodes/*.zip` as node archives
- safely extract nested node archives into temporary workspace storage
- reject unsafe archive paths and links using the same archive safety boundary as top-level ingestion
- preserve logical provenance as `nodes/<node>/bundle/<path>`
- add extracted node artifacts to the common inventory without modifying the original bundle
- make node archive extraction failures visible as warnings rather than silently discarding them
- add a first-class `chute node INPUT NODE` projection
- copy node-local artifacts into `node_bundle/` in the node case

### 3. Broad identifiers could mix evidence from adjacent volume activity

The real bundle represented an end-to-end test workflow in which an older volume and a newer volume appeared near the same workload activity. The original implementation treated every related identifier as equivalent for log selection.

An exact Longhorn volume ID and a Pod name are not equally strong evidence.

Impact:

- a line selected because it mentioned the workload could contain another volume
- downstream AI could mistake contextual adjacency for direct evidence
- the package did not communicate why a line had been selected

Fix:

Chute now assigns identifiers to evidence tiers:

```text
PRIMARY
  Longhorn volume name/UID
  engine name/UID
  replica name/UID
  CSI VolumeAttachment name/UID

SECONDARY
  PersistentVolume name/UID
  PersistentVolumeClaim name/UID

CONTEXTUAL
  Pod name/UID
  Kubernetes node name/UID
  Longhorn node name/UID
```

Every matching log window now records its strongest evidence tier and the identifiers that caused the match. Matching lines are marked inside each context window. Counts by tier are written to `evidence.json` and `summary.md`.

Contextual evidence is retained because it can be operationally important. It is no longer presented as equivalent to an exact storage identity match.

## Additional improvements made during the same hardening pass

### Node-oriented projection

New command:

```bash
chute node supportbundle.zip <node-name>
```

The case includes node objects, InstanceManagers, Pods, Engines, Replicas, VolumeAttachments, related Volumes, related Events, matching logs, timeline output, provenance, and extracted node-bundle artifacts.

### Deterministic evidence timeline

Volume and node cases now include:

```text
timeline.md
timeline.jsonl
```

The timeline combines:

- related Kubernetes Event timestamps
- timestamped log lines that directly matched an evidence identifier

It orders observed evidence only. It does not infer causation or declare a root cause.

### Coverage and warnings

Every full `process` run now writes:

```text
coverage.json
warnings.json
```

Coverage reports how much of the support bundle Chute actually interpreted, including:

- files discovered
- YAML files
- logs analyzed
- rotated logs analyzed
- node archives found and extracted
- node-archive files
- resources parsed
- unknown artifacts
- parse failures
- unsupported artifacts

Warnings make partial processing explicit instead of allowing missing evidence to look like successful analysis.

## Acceptance rule going forward

A support case is not considered trustworthy merely because Chute produced output.

For every new real-bundle shape we encounter:

1. identify the unsupported or ambiguous evidence boundary
2. document why it matters to a support investigation
3. make the smallest deterministic correction
4. add a minimized regression fixture that reproduces the structure without committing the real support bundle
5. keep diagnosis outside the deterministic preprocessing layer
