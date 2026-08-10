# Kubikles Accelerator security

Kubikles Accelerator is referred to here as Accelerator. Its security boundary protects desktop and creator credentials, Secret values, exact workload identity, and immutable release identity. Trusted actors are the local desktop user, the selected Kubernetes control plane, the cluster node/runtime, and the authorized release workflow.

## Trust boundaries

| Boundary | Control | Evidence owner | Residual risk |
|---|---|---|---|
| Desktop to workload | OS-assigned loopback tunnel fenced to exact Pod name/UID; authenticated creator session | 40C / 60A | A compromised desktop can use its own cluster authority |
| Workload to Kubernetes API | Projected ServiceAccount identity and fixed Secret-only `get/list/watch` client | 10A, 10B, 31 / 60A | The role can read every Secret in every namespace |
| Release to desktop | Descriptor checksum, literal version, immutable image/chart digests | 32, 40A / 60B | Runtime clusters do not perform admission verification |

## Threats and controls

- Generic method escalation is denied by the six-entry policy, capability checks, trusted call-context injection, and generated-dispatch exclusions. All mutations and non-Secret reads stay Direct.
- Creator credentials remain memory-only, are authenticated for the exact workload, and are destroyed during cleanup.
- Secret disclosure is reduced by value-free list/watch projections and detail-only value responses. Key names, labels, annotations, raw objects, and last-applied content do not cross list/watch/report boundaries.
- Stale workload confusion is fenced by release ownership labels, session generation, exact Pod UID, descriptor identity, and one replacement maximum.
- Denial of service is bounded by request timeouts, websocket queue/frame/read limits, watch ownership, session expiry, Job resource limits, and Direct fallback. It is not a performance or availability guarantee.

## Lifetime and resource boundaries

Zero authenticated clients start an exact two-minute grace; reconnection cancels it. The Job has a 30-second termination grace and a one-hour completion TTL. Resource requests are `100m` CPU and `128Mi` memory; limits are `1` CPU and `512Mi` memory.

Cluster-wide Secret read is a substantial blast radius. The role is minimal for the v1 all-namespace Secret read contract, not generally least privilege. A compromised desktop, node, control plane, or authorized ServiceAccount can expose values within its authority; Accelerator does not provide hardware isolation, multi-user isolation, or protection from those principals.

## Non-goals

Accelerator is not a generic proxy, mutation service, public server, HA service, compatibility bridge, or user-configurable authorization system. It does not promise universal Kubernetes compatibility, latency improvement, attack resistance, or automatic recovery under every failure. Registry admission enforcement is outside the product; release verification evidence is documented separately.

[Operations and release verification](operations.md) · [Acceptance evidence](evidence.md) · [Back to overview](../features/accelerator.md)
