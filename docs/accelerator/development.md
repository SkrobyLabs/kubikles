# Kubikles Accelerator maintainer contract

Kubikles Accelerator is referred to here as Accelerator. Public documentation uses that name; established technical identifiers such as `pkg/agent`, `AgentRouter`, and `AuthenticatedCallContext` remain source-only names.

## Exact operation boundary

| Method | Capability | Kubernetes action |
|---|---|---|
| `CancelListRequest` | `secrets.list` | no Kubernetes action |
| `GetSecretData` | `secrets.detail` | `get` |
| `GetSecretYaml` | `secrets.detail` | `get` |
| `ListSecretsMetadata` | `secrets.list` | `list` |
| `SubscribeSecretWatcher` | `secrets.watch` | `watch` |
| `UnsubscribeSecretWatcher` | `secrets.watch` | no Kubernetes action |

A generated App/Wails method does not authorize or accelerate itself. Trusted 05C context injection crosses the generated dispatcher boundary; `//kubikles:dispatch exclude` keeps internal/session-bearing entry points out of ordinary dispatch.

## Ownership map

- `pkg/agent` owns compatibility, policy, authenticated call context, and idle lifecycle contracts.
- `pkg/k8s` owns the fixed in-cluster context, Secret projection, and value-free watch behavior.
- `pkg/server` owns loopback HTTP/WebSocket authentication, creator sessions, and idle expiry.
- `pkg/acceleratorsecret` owns the typed six-operation RPC contract; root integrated router/lifecycle files and `pkg/acceleratorprovision` own Direct fallback and workload coordination.
- `deploy/charts/kubikles-accelerator`, `Dockerfile.accelerator`, `pkg/acceleratorrelease`, and the release schema/helper own deployment and immutable artifacts.
- `test/accelerator/acceptance-v1.json` with `internal/acceleratoracceptance` owns 60A functional/security evidence. `security/accelerator-toolchain.json` and `scripts/accelerator-supply-chain` own 60B evidence.

The machine-readable [documentation contract](contract-v1.json) contains the complete 27-plan title/group/focused-target map. It is checked against merged source rather than copied from work-queue state.

## Safe extension checklist

Widening the boundary requires a deliberate update to policy, capability resolution, projection/watch privacy, typed client/router, chart RBAC when applicable, the 60A manifest, this docs contract, focused tests, supply-chain impact, and freshly captured evidence. A change is incomplete if any owner remains stale. Exact compatibility and fail-closed Direct fallback stay mandatory unless the product contract itself is redesigned.

Canonical terminal gates are:

```sh
make test-accelerator-e2e
make test-accelerator-supply-chain
make test-accelerator-docs
```

[Architecture](architecture.md) · [Security](security.md) · [Back to overview](../features/accelerator.md)
