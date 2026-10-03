# Chute usage guide

This is the operator and developer workflow for running Chute against a Longhorn support bundle.

## 1. Build and verify Chute

```bash
git clone https://github.com/jenksed/chute-go.git
cd chute-go

go test ./...
go vet ./...
go build -o chute ./cmd/chute
```

Verify the binary exists:

```bash
ls -lh ./chute
```

## 2. Keep real support bundles out of Git

A support bundle can contain infrastructure names, configuration, tokens, credentials, customer data, or other sensitive material.

For manual testing, create a local-only workspace:

```bash
mkdir -p testdata/manual
printf '\n/testdata/manual/\n' >> .git/info/exclude
```

Move or copy your bundle there:

```bash
mv ~/Downloads/<support-bundle>.zip testdata/manual/test-support-bundle.zip
```

Check that Git does not see it:

```bash
git status
```

## 3. Start with inspect

```bash
./chute inspect testdata/manual/test-support-bundle.zip
```

The output is a compact table of discovered Longhorn volumes and the PVC/workload state Chute could resolve.

Use this command to answer:

- which Longhorn volumes are present?
- which PVC is each volume bound to?
- which namespace is involved?
- what state and robustness were observed?
- how many replicas and Pods are related?

If the volume you expect is missing, do not move directly to AI analysis. First inspect bundle coverage and parser behavior.

## 4. Create the narrowest useful case

### Known Longhorn volume

```bash
./chute volume --output testdata/manual/case testdata/manual/test-support-bundle.zip <LONGHORN_VOLUME>
```

### Known PVC

```bash
./chute volume --pvc <NAMESPACE/PVC> --output testdata/manual/case testdata/manual/test-support-bundle.zip
```

### Known Pod

```bash
./chute volume --pod <NAMESPACE/POD> --output testdata/manual/case testdata/manual/test-support-bundle.zip
```

A Pod selector is accepted only when Chute can resolve it to exactly one PVC-backed volume. If the Pod uses more than one PVC, use `--pvc`.

## 5. Read a volume case

Start here:

```bash
cat testdata/manual/case/summary.md
cat testdata/manual/case/timeline.md
less testdata/manual/case/relevant_logs.log
```

Then inspect machine-readable evidence:

```bash
jq . testdata/manual/case/evidence.json
jq . testdata/manual/case/sources.json
```

Then inspect the actual objects as needed:

```bash
cat testdata/manual/case/volume.yaml
cat testdata/manual/case/pv.yaml
cat testdata/manual/case/pvc.yaml
cat testdata/manual/case/volume_attachments.yaml
cat testdata/manual/case/engines.yaml
cat testdata/manual/case/replicas.yaml
cat testdata/manual/case/kubernetes_nodes.yaml
cat testdata/manual/case/longhorn_nodes.yaml
cat testdata/manual/case/events.yaml
```

## 6. Pivot to a node when the volume evidence points there

If the volume case shows attach failure, node-not-ready state, InstanceManager problems, replica locality, or other node-specific evidence:

```bash
./chute node --output testdata/manual/node-case testdata/manual/test-support-bundle.zip <NODE_NAME>
```

Read it the same way:

```bash
cat testdata/manual/node-case/summary.md
cat testdata/manual/node-case/timeline.md
less testdata/manual/node-case/relevant_logs.log
```

If the source bundle contained `nodes/<node>.zip`, Chute safely extracts it and copies its files into:

```text
node-case/node_bundle/
```

Those files are still raw evidence. Chute does not reinterpret arbitrary node files unless they are already supported by the common parsers.

## 7. Process every volume when you need bundle-wide projections

```bash
rm -rf testdata/manual/processed

./chute process --output testdata/manual/processed testdata/manual/test-support-bundle.zip
```

This is useful when:

- you do not yet know which volume matters
- you want comparable packages for every volume
- you want coverage and warning reports for the entire bundle

Inspect:

```bash
jq . testdata/manual/processed/coverage.json
jq . testdata/manual/processed/warnings.json

find testdata/manual/processed/volumes -mindepth 1 -maxdepth 1 -type d -print
```

## 8. Treat coverage as part of correctness

`coverage.json` answers whether Chute actually processed the bundle shapes it knows about.

Important fields include:

- files discovered
- YAML files
- logs analyzed
- rotated logs analyzed
- node archives found
- node archives extracted
- files discovered inside node archives
- resources parsed
- unknown artifacts
- parse failures
- unsupported artifacts
- warnings

A nonzero unknown or unsupported count is not automatically a failure. It means the support engineer should decide whether those artifacts matter to the current case.

A node archive extraction failure is more significant because node-local evidence may be missing.

## 9. Evidence tiers

Chute's log matching uses related object identities, but it does not flatten them into one undifferentiated list.

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
- related workload/node identities

The tier for a log line is the strongest identifier that actually matched that line.

Context lines surrounding a hit are included for readability but are not themselves promoted to timeline entries unless they independently match an evidence identifier.

## 10. Change log context deliberately

Default:

```text
5 lines before
matching line
5 lines after
```

Increase it when the component log requires more local state:

```bash
./chute volume --context 20 test-support-bundle.zip <LONGHORN_VOLUME>
```

Set it to zero when you only want direct matching lines:

```bash
./chute volume --context 0 test-support-bundle.zip <LONGHORN_VOLUME>
```

Larger context does not increase relationship accuracy. It only includes more surrounding text.

## 11. Feeding a case to an LLM

The useful unit is the generated case directory, not the original support bundle.

A reasonable order is:

1. `summary.md`
2. `timeline.md`
3. `evidence.json`
4. `relevant_logs.log`
5. specific YAML objects only when needed
6. node-local files only when the case has pivoted to a node

Do not tell the model that Chute found the root cause. Chute only selected and organized evidence.

A useful instruction is:

```text
Analyze this case using only the supplied evidence.
Separate observed facts from hypotheses.
Cite source files and line ranges when making claims.
Do not treat contextual evidence as proof that another object belongs to the selected volume.
```

## 12. Re-run after changing Chute

The minimum local acceptance gate is:

```bash
go test ./...
go vet ./...
go build ./cmd/chute
```

Then rerun the same real support bundle and compare:

```bash
rm -rf testdata/manual/processed

./chute process --output testdata/manual/processed testdata/manual/test-support-bundle.zip
```

Real-bundle compatibility changes should be driven by an observed bundle shape, then captured in a minimized regression fixture. Do not commit the original bundle.

For a concrete end-to-end investigation using only values observed from the first real validation bundle, see [EXAMPLE_CASE.md](EXAMPLE_CASE.md).
