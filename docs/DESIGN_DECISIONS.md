# Chute design decisions

This document records decisions that should remain stable unless real support-bundle evidence gives us a reason to change them.

## 1. Chute is an evidence compiler, not a diagnostic engine

Chute parses, relates, filters, orders, and packages observed support-bundle evidence.

It does not:

- declare root cause
- score likely diagnoses
- rewrite observed data into inferred facts
- require an LLM to produce its core output

Reason: deterministic evidence preparation is independently testable and useful to both humans and AI systems.

## 2. Go is the implementation language

Go is used because Chute is a standalone CLI adjacent to Longhorn/Kubernetes operational tooling.

The decision optimizes for:

- single-binary distribution
- contributor familiarity in the surrounding ecosystem
- straightforward filesystem/archive processing
- future compatibility with Go-based Kubernetes/Longhorn libraries when justified

The decision is not based on a claim that support-bundle transformation requires Go.

## 3. Raw artifacts remain authoritative

Generated summaries and relationships never replace the source artifact.

Every projected resource retains source provenance. Log evidence retains source path and line ranges. Node archive contents retain a logical path under the originating node.

## 4. Original inputs are never mutated

Top-level archives and nested node archives are extracted into temporary workspace storage.

Directory inputs are read in place, but nested archives are still extracted elsewhere.

Reason: support bundles are evidence. Chute should not alter the evidence it is analyzing.

## 5. Archive extraction is fail-closed for unsafe paths

Path traversal and archive links are rejected.

A malicious or malformed archive must not be able to write outside the extraction workspace.

Top-level archive safety violations fail the command. A nested node archive that cannot be safely extracted is omitted and reported as a warning so the rest of the support bundle remains usable.

## 6. Nested node archives are evidence sources, not a separate product

Files extracted from `nodes/<node>.zip` join the common inventory with:

- `origin=node_archive`
- the associated node name
- a logical source path under `nodes/<node>/bundle/`
- an internal physical source path used only while processing

This lets existing parsing and log-selection machinery operate across top-level and node-local evidence without modifying the original bundle.

## 7. Evidence identifiers have explicit strength

Identifier tiers are:

### Primary

Exact storage identities:

- Longhorn Volume
- Engine
- Replica
- CSI VolumeAttachment

### Secondary

Bound Kubernetes storage identities:

- PersistentVolume
- PersistentVolumeClaim

### Contextual

Operational neighbors:

- Pod
- Node
- other workload/node identities used by node-oriented cases

Reason: a workload or node mention may be important context but does not prove that a log line is directly about a specific volume.

Chute retains contextual evidence but labels it.

## 8. Context windows are bounded and merged

A matching log line includes a configurable number of lines before and after it.

Overlapping windows are merged to avoid duplicate evidence.

A hard global line limit remains in place to prevent an evidence package from becoming an unbounded log dump.

## 9. Rotated and compressed logs are first-class logs

Chute recognizes:

```text
*.log
*.log.N
*.log.gz
*.log.N.gz
```

Files in a logical `logs/` tree are also treated as logs.

Gzip logs are decompressed while reading; the source artifact itself is unchanged.

## 10. Timelines order evidence but do not create causal claims

Timeline entries come from:

- related Kubernetes Events with parseable timestamps
- log lines that matched an evidence identifier and contain a parseable RFC3339 timestamp

Timeline ordering is deterministic. It is not a root-cause analysis.

Lines included only as surrounding context do not become timeline events unless they themselves match an evidence identifier.

## 11. Node projection is first-class because failures cross the volume/node boundary

A volume case can establish that a target node is implicated.

A node case provides the inverse projection: node objects, InstanceManagers, workloads, storage resources, node-local artifacts, events, logs, and timeline.

This avoids forcing one oversized case format to represent every support investigation.

## 12. Coverage is part of correctness

Successful execution does not mean complete evidence coverage.

`coverage.json` and `warnings.json` explicitly report what Chute understood and what it did not.

Unknown artifacts remain inventoried. They are not silently discarded or described as analyzed.

## 13. Real support bundles do not become test fixtures

Real bundles can contain credentials, customer data, infrastructure names, and other sensitive material.

Regression tests reproduce only the structural condition that caused a bug or gap.

The first real-bundle hardening fixture models:

- a root-relative rotated log
- a nested node ZIP
- a node readiness transition
- an InstanceManager on the affected node
- two volumes appearing around shared workload context

It does not contain the original support bundle.

## 14. Compatibility changes are evidence-driven

Chute does not add speculative format handlers because a format might exist.

A new parser, relationship rule, archive type, or heuristic should be justified by:

- a real support bundle
- Longhorn's documented bundle format
- or an upstream test artifact that demonstrates the need

This keeps the deterministic core small and reviewable.


## 15. Identifier matching uses resource boundaries, not arbitrary substrings

Evidence matching is literal but boundary-aware.

Kubernetes/Longhorn identifier characters are treated as alphanumeric characters plus `-`, `_`, and `.`. A candidate identifier only matches when the characters immediately before and after it are not part of another identifier.

Reason: during CI for the real-bundle hardening pass, the PVC name `data` matched the `data` prefix inside the Pod name `database-0`. That incorrectly upgraded a contextual workload line to secondary evidence.

Boundary-aware matching keeps exact identities useful without allowing short resource names to match unrelated larger names.
