# Chute

Chute is a deterministic Longhorn support-bundle slicer.

Give it a Longhorn support bundle and a resource you care about, usually a volume, PVC, Pod, or node. Chute parses the bundle, follows explicit Kubernetes and Longhorn relationships, selects related logs and events, and writes a much smaller evidence directory with provenance.

It does not diagnose the problem. It prepares the evidence so a human or an LLM does not have to rediscover the topology of the bundle every time.

## The problem

A Longhorn support bundle is useful, but it is not organized like a support case.

Evidence for one failed volume operation can be spread across:

- Longhorn Volume, Engine, Replica, Node, and InstanceManager objects
- Kubernetes PV, PVC, Pod, Node, Event, and VolumeAttachment objects
- current logs
- rotated logs
- compressed logs
- nested per-node support archives
- files that are unrelated to the affected volume

A raw bundle answers:

> What did the cluster export?

A support engineer usually needs:

> What evidence is connected to this volume or node, where did it come from, and what happened around the same time?

Chute performs that transformation deterministically.

## What Chute actually does

The core pipeline is:

```text
support bundle
    |
    v
safe archive extraction
    |
    v
file inventory
    |
    +--> YAML resource parsing
    +--> current / rotated / compressed log discovery
    +--> nested node archive ingestion
    |
    v
resource index
    |
    v
explicit relationship projection
    |
    +--> volume case
    |      Volume -> PV/PVC -> Pod
    |             -> VolumeAttachment
    |             -> Engine/Replica
    |             -> Nodes
    |             -> Events
    |
    +--> node case
           Node -> InstanceManagers
                -> Pods
                -> Engines/Replicas
                -> VolumeAttachments
                -> Volumes
                -> node-local artifacts
    |
    v
bounded evidence package
    +--> source YAML
    +--> relevant log windows
    +--> events
    +--> timeline
    +--> provenance
    +--> evidence-strength labels
```

No embeddings, vector database, agent loop, or LLM is required.

## Why not just grep the bundle?

You can, but grep does not know the object graph.

For example:

- a PVC name may point to a PV whose CSI `volumeHandle` is the Longhorn volume ID
- a Pod may identify the workload using that PVC
- a VolumeAttachment may identify the target node
- Longhorn Engine and Replica objects may identify the nodes actually hosting volume processes
- a short resource name must not be treated as a match when it only appears inside a longer unrelated resource name
- a useful failure may live in `longhorn-manager.log.1`, not the current log
- the useful kubelet or mount evidence may be inside `nodes/<node>.zip`

Chute resolves those relationships first, then searches using the resulting evidence identities.

## Build

Requirements:

- Go 1.23 or compatible newer Go toolchain
- no Longhorn cluster access required
- no network access required during analysis after dependencies are available

```bash
git clone https://github.com/jenksed/chute-go.git
cd chute-go

go test ./...
go vet ./...
go build -o chute ./cmd/chute
```

The built `chute` executable is the CLI.

## Fastest path for a real support case

Assume:

```text
test-support-bundle.zip
```

### 1. See what volumes Chute found

```bash
./chute inspect test-support-bundle.zip
```

This is usually the first command to run. For the actual January 5 validation bundle, see [the worked example](docs/EXAMPLE_CASE.md).

### 2. Build a case for one volume

By Longhorn volume name:

```bash
./chute volume --output ./case test-support-bundle.zip <LONGHORN_VOLUME>
```

By PVC:

```bash
./chute volume --pvc <NAMESPACE/PVC> --output ./case test-support-bundle.zip
```

By Pod:

```bash
./chute volume --pod <NAMESPACE/POD> --output ./case test-support-bundle.zip
```

If a Pod uses multiple PVCs, Chute refuses to guess. Select the PVC explicitly.

### 3. Read these files first

```text
case/
├── summary.md
├── timeline.md
├── relevant_logs.log
├── evidence.json
└── sources.json
```

Use them in that order:

1. `summary.md` tells you what Chute related to the case.
2. `timeline.md` orders timestamped matching evidence.
3. `relevant_logs.log` gives bounded log context with source paths and evidence tiers.
4. `evidence.json` gives the machine-readable case model.
5. `sources.json` tells you where exported evidence came from.

The remaining YAML files are the actual related objects.

### 4. If the case points at a node, pivot to the node

```bash
./chute node --output ./node-case test-support-bundle.zip <NODE_NAME>
```

A node case includes node objects, InstanceManagers, Pods, Engines, Replicas, VolumeAttachments, Volumes, Events, matching logs, a timeline, and node-local artifacts extracted from `nodes/<node>.zip` when present.

## Process the whole bundle

If you want one projected directory for every Longhorn volume:

```bash
./chute process --output ./processed test-support-bundle.zip
```

This also writes bundle-level coverage information:

```text
processed/
├── manifest.json
├── index.json
├── coverage.json
├── warnings.json
└── volumes/
    ├── <volume-a>/
    └── <volume-b>/
```

Do not assume a successful command means Chute understood every file. Check `coverage.json` and `warnings.json`.

## Log context

Chute includes five lines before and after each matching log line by default.

Change it with `--context`:

```bash
./chute volume --context 10 test-support-bundle.zip <LONGHORN_VOLUME>
./chute node --context 10 test-support-bundle.zip <NODE_NAME>
./chute process --context 0 --output ./processed test-support-bundle.zip
```

Overlapping windows are merged. Chute also enforces a global evidence-line limit so a case does not turn back into an unbounded log dump.

## Evidence strength

Not every related string is equally strong evidence.

Chute labels identifiers as:

```text
PRIMARY
  Longhorn Volume
  Longhorn Engine
  Longhorn Replica
  CSI VolumeAttachment

SECONDARY
  PersistentVolume
  PersistentVolumeClaim

CONTEXTUAL
  Pod
  Node
  other operational neighbors
```

A log line that contains an exact Longhorn volume ID is stronger evidence for that volume than a line that only contains the Pod name.

Chute keeps contextual evidence because it may explain sequence or neighboring activity, but it labels it so downstream analysis does not silently treat adjacency as identity.

Identifier matching is also boundary-aware, so a short resource name does not match merely because its characters occur inside a longer resource identifier.

## Supported input

```text
extracted directory
.zip
.tar.gz
.tgz
```

Chute also recognizes:

```text
*.log
*.log.N
*.log.gz
*.log.N.gz
```

Nested `nodes/<node>.zip` archives are extracted into temporary workspace storage and added to the evidence inventory.

The original bundle is never modified.

Archive path traversal and archive links are rejected.

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

See [docs/OUTPUT_FORMAT.md](docs/OUTPUT_FORMAT.md) for the semantics of each file.

## What Chute does not do

Chute does not:

- declare a root cause
- rank diagnoses
- call an LLM
- mutate the source bundle
- hide unknown artifacts
- assume contextual correlation means causation
- require access to the original Kubernetes cluster

That boundary is deliberate. Chute's job is to produce a smaller, inspectable, reproducible evidence package.

## When I would use it

Use Chute when you have a Longhorn support bundle and one of these is true:

- you know the affected PVC, Pod, Longhorn volume, or node
- you need to hand a bounded case to another engineer
- you want to give an LLM relevant evidence without dumping the entire support bundle into context
- you need explicit provenance for why an object or log block is in the case
- you need to know whether rotated logs or node-local evidence were actually processed
- you are comparing repeated support cases and want the preprocessing step to be deterministic

If you need live cluster interrogation, automated diagnosis, or remediation, Chute is the wrong layer.

## More detail

- [Worked example from a real validation bundle](docs/EXAMPLE_CASE.md)
- [Usage guide](docs/USAGE.md)
- [Output format](docs/OUTPUT_FORMAT.md)
- [Design decisions](docs/DESIGN_DECISIONS.md)
- [First real-bundle validation](docs/REAL_BUNDLE_VALIDATION.md)

The real support bundle used during validation is not committed. Regression tests reproduce the structural failure modes with minimized fixture data.
