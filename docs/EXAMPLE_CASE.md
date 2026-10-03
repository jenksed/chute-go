# Worked example: January 5, 2026 Longhorn test bundle

This is a real usage example from the first non-synthetic support bundle used to validate Chute.

The original support bundle is not committed to this repository. The commands below assume the local copy was renamed to:

```text
testdata/manual/test-support-bundle.zip
```

This document only records things we actually observed from that bundle. It intentionally stops short of inventing a final root cause or resolution that we have not established.

## 1. The bundle contained two Longhorn volumes

A full processing run produced two volume case directories:

```text
testdata/manual/processed/volumes/pvc-ec25952c-fc84-4126-a5fb-9e1746e87e0e
testdata/manual/processed/volumes/pvc-ed2ef385-8963-4a97-95fb-45fd8640b9db
```

That immediately gives an engineer a useful boundary: this is not a bundle with hundreds of candidate Longhorn volumes.

The normal first command is:

```bash
./chute inspect testdata/manual/test-support-bundle.zip
```

Then either process the entire bundle:

```bash
./chute process --output testdata/manual/processed testdata/manual/test-support-bundle.zip
```

or go directly to a known volume.

## 2. One projected volume had a coherent storage/workload graph

The first real volume we examined was:

```text
pvc-ec25952c-fc84-4126-a5fb-9e1746e87e0e
```

Create only that case:

```bash
./chute volume --output testdata/manual/case testdata/manual/test-support-bundle.zip pvc-ec25952c-fc84-4126-a5fb-9e1746e87e0e
```

The initial real-bundle run resolved this observed topology:

```text
Longhorn Volume
  pvc-ec25952c-fc84-4126-a5fb-9e1746e87e0e
  state: detached
  robustness: unknown
        |
        +--> PersistentVolume
        |     pvc-ec25952c-fc84-4126-a5fb-9e1746e87e0e
        |
        +--> PersistentVolumeClaim
        |     pod-data-e2e-test-statefulset-0-0
        |
        +--> Engine: 1
        +--> Replicas: 3
        +--> Pod: 1
        +--> VolumeAttachment: 1
        +--> Kubernetes Nodes: 3
        +--> Longhorn Nodes: 3
```

Those values came directly from the generated `summary.md` for the real bundle.

This is the main value of Chute: the engineer does not need to manually search the bundle for each resource and determine whether it belongs to the same case.

## 3. Start by reading the generated case, not the raw bundle

For this volume:

```bash
cat testdata/manual/case/summary.md
cat testdata/manual/case/timeline.md
less testdata/manual/case/relevant_logs.log
jq . testdata/manual/case/evidence.json
```

The initial pre-hardening run found:

```text
17 matching identifiers
456 matching log lines
26 evidence windows
1027 included log lines
```

These counts are historical observations from the first run. They are not golden expected values.

After the real-bundle hardening work, counts may change because Chute now:

- recognizes rotated logs that were previously skipped
- ingests nested node archives
- uses boundary-aware identifier matching
- distinguishes primary, secondary, and contextual evidence

A change in those counts is not automatically a regression. The useful invariant is that selected evidence remains traceable to the selected case and Chute reports what it actually processed.

## 4. The volume evidence pointed at a specific node

The selected evidence showed repeated attach attempts for:

```text
pvc-ec25952c-fc84-4126-a5fb-9e1746e87e0e
```

targeting:

```text
ip-10-0-1-181
```

The relevant evidence included failures reporting that the target node was not ready.

Later evidence in the same support bundle showed the node transitioning to Ready, followed by an InstanceManager-not-running condition, and later additional node warning/down evidence.

That is enough to justify a node pivot.

It is not enough to declare a root cause.

Generate the node case:

```bash
./chute node --output testdata/manual/node-ip-10-0-1-181 testdata/manual/test-support-bundle.zip ip-10-0-1-181
```

Then inspect:

```bash
cat testdata/manual/node-ip-10-0-1-181/summary.md
cat testdata/manual/node-ip-10-0-1-181/timeline.md
less testdata/manual/node-ip-10-0-1-181/relevant_logs.log
find testdata/manual/node-ip-10-0-1-181/node_bundle -type f | sort
```

This is the intended investigation flow:

```text
raw support bundle
       |
       v
volume case
       |
       v
attach failure associated with ip-10-0-1-181
       |
       v
node case
       |
       +--> Kubernetes / Longhorn Node
       +--> InstanceManagers
       +--> related storage resources
       +--> related events
       +--> matching logs
       +--> extracted node-local bundle evidence
```

## 5. The second volume exposed why evidence strength matters

The same real bundle also contained:

```text
pvc-ed2ef385-8963-4a97-95fb-45fd8640b9db
```

Activity involving that volume appeared around the same test-workload context as the selected `pvc-ec259...` volume.

The original implementation treated all related names as equivalent log selectors. That meant a workload-name match could pull neighboring activity involving the other volume into the same evidence package without saying how direct the relationship was.

That real observation is why Chute now labels evidence:

```text
PRIMARY
  exact Longhorn Volume / Engine / Replica / VolumeAttachment identities

SECONDARY
  PV / PVC identities bound to the selected volume

CONTEXTUAL
  Pod / Node / operational-neighbor identities
```

Contextual evidence is still useful. The label prevents it from being mistaken for exact volume identity.

## 6. Check coverage before trusting the case as complete

The real bundle also exposed two types of evidence the first implementation was not processing correctly:

- rotated `.log.1` files
- nested `nodes/<node>.zip` archives

After processing the bundle, inspect:

```bash
jq . testdata/manual/processed/coverage.json
jq . testdata/manual/processed/warnings.json
```

The point is not to force every unknown-artifact count to zero.

The point is to know whether potentially relevant evidence was omitted.

For this case, node-local evidence matters because the volume evidence specifically implicated a node.

## 7. What we can say from this example

Observed:

- the bundle contained two Longhorn volumes
- Chute resolved the selected detached volume to its PV, PVC, Pod, Engine, Replicas, VolumeAttachment, and nodes
- selected logs showed attach failures involving `ip-10-0-1-181`
- evidence showed node readiness changes
- evidence showed an InstanceManager-not-running condition
- the bundle contained rotated logs
- the bundle contained nested node support archives
- shared workload context could include activity involving another volume

Not established by this example:

- the final root cause
- the final human support conclusion
- the exact remediation
- whether any single node or InstanceManager condition alone caused the complete incident

That distinction is intentional. Chute should make the evidence boundary sharper, not manufacture certainty.

## 8. How this example should evolve

When we validate another real bundle, add another worked example only when it teaches a different investigation path.

Useful future examples would be real cases such as:

```text
volume -> replica problem
volume -> disk / node problem
volume -> attachment problem
node -> InstanceManager problem
PVC / Pod selector -> resolved Longhorn volume
```

Do not invent those cases for documentation. Add them when an actual support bundle demonstrates them.
