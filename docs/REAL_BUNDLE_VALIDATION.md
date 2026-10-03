# Real-bundle validation: January 5, 2026 Longhorn test bundle

This document records the first validation of Chute against a real Longhorn support bundle and the changes that validation drove.

The source bundle used for validation was kept local and is not committed to this repository. Regression tests reproduce the relevant structure with minimized fixture data.

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

The resulting evidence package exposed repeated CSI attach failures to a node Longhorn reported as not ready.

Later evidence showed node readiness transitions and InstanceManager readiness failures.

That validated the core resource-graph approach: deterministic relationship projection was able to turn the raw bundle into a coherent support case without an AI model deciding which files were related.

## Issues discovered

### 1. Rotated logs were silently omitted

The real bundle contained files such as:

```text
logs/.../longhorn-manager.log.1
logs/.../csi-attacher.log.1
```

The original classifier only recognized a final `.log` extension and checked for `/logs/` inside a root-relative path.

A path beginning with `logs/` therefore did not satisfy the directory check, and `.log.1` did not have a `.log` extension.

Impact:

- historical evidence could be excluded from every case
- failures immediately before rotation could disappear from the projected case
- the operator had no direct indication that those files were skipped

Fix:

- normalize logical paths before directory classification
- recognize `.log`, `.log.N`, `.log.gz`, and `.log.N.gz`
- read gzip-compressed logs transparently
- report rotated-log coverage in `coverage.json`
- add regression coverage for a root-relative `logs/.../*.log.1`

### 2. Nested node support bundles were opaque

The real support bundle contained one ZIP archive per node under `nodes/`.

The node involved in the attach failure had its own nested archive, but Chute originally inventoried that ZIP as unknown and did not inspect it.

Impact:

- Chute could establish that attach failed because the node was not ready
- it could not surface node-local evidence that might explain why
- kubelet, mount, disk, kernel, and service evidence could remain hidden

Fix:

- classify `nodes/*.zip` as node archives
- safely extract them into temporary workspace storage
- reject unsafe archive paths and links
- preserve logical provenance under `nodes/<node>/bundle/<path>`
- add extracted node artifacts to the common inventory
- keep the original bundle unchanged
- report failed nested archive ingestion as a warning
- add `chute node INPUT NODE`
- copy node-local evidence into `node_bundle/` in node cases

### 3. Broad identifiers could mix evidence from adjacent volume activity

The real bundle represented an end-to-end test workflow in which an older volume and a newer volume appeared around the same workload activity.

The first implementation treated all related identifiers as equivalent log selectors.

An exact Longhorn volume ID and a Pod name are not equally strong evidence.

Impact:

- a line selected because it mentioned the workload could also contain another volume
- downstream analysis could mistake contextual adjacency for direct volume evidence
- the output did not explain why a line had been selected

Fix:

Chute now assigns evidence tiers:

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

Every matching log block records:

- the strongest tier represented
- the identifiers that caused selection
- exact matching lines
- surrounding bounded context

Contextual evidence is retained, but it is no longer presented as equivalent to a direct storage identity match.

### 4. Evidence tiering exposed a substring-matching defect

The first CI run of the tier implementation found another precision problem.

A short PVC name such as `data` could match inside a longer Pod name such as `database-0`.

That incorrectly promoted a contextual Pod line to secondary evidence.

Impact:

- tier labels could exist but still be wrong
- short resource names could generate false-positive matches
- downstream analysis could trust an incorrectly strengthened relationship

Fix:

- identifier matching is boundary-aware rather than arbitrary substring matching
- alphanumeric characters, `-`, `_`, and `.` are treated as identifier characters
- the minimized regression fixture asserts that shared-workload context remains contextual

The defect was found and fixed before the hardening branch was merged.

## Additional changes made during the hardening pass

### Node-oriented projection

New command:

```bash
chute node supportbundle.zip <node-name>
```

A node case includes:

- Kubernetes Node
- Longhorn Node
- InstanceManagers
- Pods
- Engines
- Replicas
- VolumeAttachments
- related Volumes
- Events
- matching logs
- timeline output
- provenance
- extracted node-bundle artifacts

### Deterministic evidence timeline

Volume and node cases include:

```text
timeline.md
timeline.jsonl
```

The timeline combines:

- related Kubernetes Event timestamps
- timestamped log lines that independently matched an evidence identifier

It orders observed evidence only.

It does not infer causation or declare a root cause.

### Coverage and warnings

A full `process` run writes:

```text
coverage.json
warnings.json
```

Coverage reports how much of the support bundle Chute interpreted.

Warnings make partial processing explicit instead of allowing missing evidence to look like successful analysis.

## Acceptance rule going forward

A support case is not considered trustworthy merely because Chute produced output.

For every new real-bundle shape:

1. identify the unsupported or ambiguous evidence boundary
2. document why it matters to a support investigation
3. make the smallest deterministic correction
4. add a minimized regression fixture reproducing the structure
5. do not commit the original real support bundle
6. keep diagnosis outside the deterministic preprocessing layer

That rule is part of the design, not only a testing preference.
