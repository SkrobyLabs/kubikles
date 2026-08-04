# Kubikles Accelerator

Kubikles Accelerator is an optional, automatically managed, disposable Kubernetes Job that can move a narrowly defined set of Secret reads closer to the API server. After this formal first use, this guide calls it Accelerator. Direct access remains immediate and authoritative whenever acceleration is unavailable or unsafe.

## Choose a mode

| Mode | When it applies | What it can do | What it never does |
|---|---|---|---|
| Direct | Always, including every fallback | All ordinary desktop operations through the desktop Kubernetes client | Depend on the disposable Job |
| Integrated | Automatically after exact identity, version, authentication, and capability checks | Secret list, detail, and watch reads | Mutations or non-Secret reads |

There is no routing or RBAC mode for users to select. Integrated covers exactly three user-visible read categories. Its six technical operations are maintained in the [development contract](../accelerator/development.md#exact-operation-boundary).

## Exact compatibility and fallback

Desktop and Accelerator must report literal, nonempty `BuildVersion` equality. Versions are opaque strings: there are no ranges, semantic-version rules, N−1 support, capability compatibility bridge, or backward-compatibility promise. On the first mismatch, the desktop disposes the workload, resolves a fresh descriptor, and may create one replacement. If that replacement also mismatches, it is disposed, no third workload is created, and Direct is latched for the current desktop session or continuous-demand epoch.

Permission self-review and every provisioning, authentication, connection, or routing failure fail closed to Direct. Accelerator is an optimization, never an availability prerequisite.

## Read next

- [Architecture and lifecycle](../accelerator/architecture.md)
- [Operations and release verification](../accelerator/operations.md)
- [Security and privacy](../accelerator/security.md)
- [Maintainer contract](../accelerator/development.md)
- [Acceptance evidence](../accelerator/evidence.md)
