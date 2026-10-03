# Chute

Chute turns an extracted Longhorn support bundle into a smaller, deterministic evidence package organized around the resources an engineer actually investigates.

This is the Go implementation. V1 is an offline evidence transformer, not a Longhorn component and not a diagnostic authority.

## V1 capability

Given an extracted Longhorn support bundle, Chute can:

- inventory files and classify likely YAML, logs, node data, and unknown artifacts
- parse Kubernetes and Longhorn YAML resources
- index resources by kind and name
- connect PVCs, PVs, Pods, VolumeAttachments, Longhorn volumes, engines, replicas, and nodes using explicit identifiers
- emit a per-volume case directory containing related objects and provenance
- conservatively extract log lines containing identifiers related to the selected volume
- record unclassified files in the manifest without copying the original bundle

It does not attempt root-cause diagnosis.

## Build

```bash
go build -o chute ./cmd/chute
```

The result is a single executable with no Go runtime installation required on the target machine.

## Usage

Process every Longhorn volume found in an extracted support bundle:

```bash
./chute process --output ./processed /path/to/extracted/supportbundle
```

Project one volume into its own case directory:

```bash
./chute volume --output ./case /path/to/extracted/supportbundle pvc-abc123
```

The bundle must already be extracted in V1.

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

## Evidence boundary

Chute separates four concerns:

1. Raw bundle artifacts remain the source evidence.
2. Chute derives deterministic relationships from explicit resource identifiers.
3. Exported summaries describe observed state and related evidence.
4. Root-cause judgment remains with the engineer or downstream analysis system.

Correlation is not emitted as causation.

## Why Go

Chute lives next to Longhorn and Kubernetes operational tooling and is intended to be easy for support engineers to distribute and run. Go gives the project a small deployment surface, a single executable, and a language/toolchain familiar to the surrounding ecosystem.

That is an operational choice, not a claim that evidence transformation requires Go.

## Current acceptance boundary

The automated tests prove the synthetic vertical slice: YAML parsing, Longhorn/Kubernetes relationship projection, provenance export, bounded log filtering, and case generation.

The next meaningful acceptance step is a real Longhorn support bundle. Real bundle structure and version differences should drive the next changes rather than speculative compatibility layers.
