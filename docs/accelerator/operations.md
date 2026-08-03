# Kubikles Accelerator operations

Kubikles Accelerator is referred to here as Accelerator. It is automatically provisioned when the desktop proves exact release identity, capability, and access. Failure or denial leaves reads on Direct; operators do not choose a routing or RBAC mode.

## Runtime prerequisites and ownership

The desktop needs working Kubernetes credentials plus access to create the fixed Helm release. The chart owns exactly five objects: ServiceAccount, ClusterRole, ClusterRoleBinding, verifier Secret, and Job. The ClusterRole is cluster-wide because the v1 contract reads Secrets in all namespaces. Its only core Secret verbs are `get`, `list`, and `watch`; there are no Secret writes, workload verbs, impersonation, escalation, bind, self-review permission, or configurable permission modes.

`get` is necessary because explicit detail and YAML responses contain values. Lists and watches project only name, namespace, UID, creation timestamp, type, and data-key count. They exclude values, key names, labels, annotations, last-applied content, and raw objects. Values may exist only in the requested detail response and transient UI/client memory—not in lists, watches, progress, logs, diagnostics, reports, bundles, SBOMs, attestations, or CI artifacts.

Permission preview can be checked without reading a Secret:

```sh
kubectl auth can-i list secrets --all-namespaces --as=system:serviceaccount:example:accelerator-reader
kubectl get jobs -n example -l app.kubernetes.io/name=kubikles-accelerator -o name
helm status example-accelerator -n example
```

## Provisioning and cleanup

The desktop resolves and verifies an immutable descriptor, creates the release, observes its exact Pod, opens the loopback tunnel, authenticates, and checks capabilities. Bounded retry never makes Accelerator authoritative. Normal disposal is a desktop-owned Helm uninstall. Natural completion is cleaned by Kubernetes TTL after 3600 seconds; Accelerator does not self-delete. Startup sweep is limited to inert, owned releases.

## Release trust

The image repository is `ghcr.io/skrobylabs/kubikles-accelerator`; the chart repository is `oci://ghcr.io/skrobylabs/helm/kubikles-accelerator`. Consumers use descriptor and digest identity, not `latest`, a mutable channel, or a tag-only install. A stable tag `vX.Y.Z`, `BuildVersion` `vX.Y.Z`, and chart version `X.Y.Z` identify the same release.

Each release includes `kubikles-accelerator-release-vX.Y.Z.json` and its checksum, plus three SPDX files: the chart, linux/amd64 image, and linux/arm64 image SBOMs. Six attestations cover chart provenance/SBOM, image provenance/SBOM, and release provenance/SBOM. Vulnerability policy covers LOW through CRITICAL; HIGH/CRITICAL exceptions require the exact vulnerability ID, purl, artifact, owner, justification, and an exclusive `expiresOn` date. Verification fixes the GitHub workflow identity to this repository and release workflow.

Run the bounded checks rather than copying mutable registry commands:

```sh
make test-accelerator-release-contract
make test-accelerator-supply-chain
make verify-accelerator-release-ghcr BUILD_VERSION=vX.Y.Z
make verify-accelerator-supply-chain-ghcr BUILD_VERSION=vX.Y.Z
```

Desktop resolution verifies the descriptor checksum, image/chart digests, and `BuildVersion`. Runtime clusters do not enforce signatures, SBOMs, provenance, or vulnerability policy; those are publication and verification controls, not admission controls.

## Troubleshooting

| ID | Safe symptom or diagnostic | Automatic result | Escalate when |
|---|---|---|---|
| `ACC-CAPABILITY-DENIED` | `kubectl auth can-i list secrets --all-namespaces --as=system:serviceaccount:example:accelerator-reader` reports denial | Reads remain Direct; no expanded permission is requested | The documented fixed role should have been installed but is absent |
| `ACC-CONNECTION-LOST` | Desktop reports the bounded connection state; check metadata-only Job status | Resume is attempted only for the same workload, then Direct | The owned Job is healthy but repeatable exact-Pod connection fails |
| `ACC-DESCRIPTOR-INVALID` | Run `make test-accelerator-release-contract` | Provisioning stops and Direct remains authoritative | Published checksum, schema, or digest identity fails verification |
| `ACC-PROVISION-FAILED` | Check `kubectl get jobs -n example -l app.kubernetes.io/name=kubikles-accelerator -o name` | Rollback/disposal is bounded; Direct continues | An owned inert release remains after normal automatic cleanup |
| `ACC-VERSION-MISMATCH` | Compare the desktop status build label with the verified descriptor label | One disposal, fresh resolution, and at most one replacement | A verified descriptor repeatedly selects a different literal label |
| `ACC-REPEATED-MISMATCH` | Desktop reports the fixed repeated-mismatch state | Replacement is disposed; Direct latches; no third workload | A new desktop session still receives an inconsistent descriptor |

Diagnostics must remain metadata-only. Do not extract credentials or copy raw process output into tickets or artifacts. Ownership-sensitive cleanup remains with the desktop lifecycle.

[Back to the Accelerator overview](../features/accelerator.md)
