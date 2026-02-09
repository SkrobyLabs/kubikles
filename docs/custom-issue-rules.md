# Custom Issue Detector Rules

The Issue Detector ships with built-in rules but you can add your own by placing YAML files in the rules directory.

## Rules Directory

| OS | Path |
|----|------|
| macOS | `~/Library/Application Support/kubikles/rules/` |
| Linux | `~/.config/kubikles/rules/` |
| Windows | `%APPDATA%\kubikles\rules\` |

You can open this directory from the Issue Detector UI via the **Rules** panel > folder icon.

Files must have a `.yaml` or `.yml` extension. After adding or editing files, click **Reload** in the Rules panel (no restart needed).

## Rule Structure

```yaml
id: "USR-001"              # Unique ID (prefix with USR- to avoid future collisions)
name: "Human-readable name"
description: "Optional longer description"
severity: warning          # critical | warning | info
category: networking       # networking | workloads | storage | security | config
requires:                  # Kubernetes resources the rule needs (fetched automatically)
  - pods
  - services
check:
  type: fieldMatch         # Check type (see below)
  resource: pods           # Primary resource to inspect
  # ... type-specific fields
  message: "Pod {{.Name}} has an issue"
  suggestedFix: "Do this to fix it"
```

### Required Fields

| Field | Description |
|-------|-------------|
| `id` | Unique rule identifier. Use a `USR-` prefix for custom rules. |
| `name` | Short name shown in the UI. |
| `severity` | One of `critical`, `warning`, `info`. |
| `category` | One of `networking`, `workloads`, `storage`, `security`, `config`, `deprecation`. |
| `requires` | List of resource types the rule needs fetched. |
| `check.type` | Check type (see below). |
| `check.message` | Finding message. Supports `{{.Name}}`, `{{.Namespace}}` template vars. |

### Available Resources

`pods`, `services`, `ingresses`, `ingressclasses`, `endpoints`, `configmaps`, `secrets`, `deployments`, `statefulsets`, `daemonsets`, `pvcs`, `pvs`, `nodes`, `serviceaccounts`, `hpas`

### Field Paths

Fields are dot-separated paths into the Kubernetes JSON representation. Use `[*]` to iterate arrays.

```
.metadata.name
.metadata.namespace
.spec.containers[*].image
.spec.containers[*].resources.limits.memory
.spec.volumes[*].configMap.name
.status.conditions
```

### Message Templates

Messages support `{{.Key}}` placeholders. Available variables depend on the check type:

| Variable | Available in | Description |
|----------|-------------|-------------|
| `{{.Name}}` | All | Resource name |
| `{{.Namespace}}` | All | Resource namespace |
| `{{.RefValue}}` | `resourceExists` | The referenced value that wasn't found |
| `{{.ConditionType}}` | `statusCheck` | The condition type |
| `{{.ConditionStatus}}` | `statusCheck` | The actual condition status |
| `{{.Count}}` | `resourceCount` | Number of matching resources |

---

## Check Types

### 1. `fieldMatch` - Check a field value

Matches a field against a value using an operator. Fires when the match succeeds.

```yaml
check:
  type: fieldMatch
  resource: pods
  field: ".spec.containers[*].image"
  operator: regex           # regex | equals | notEquals
  value: ":latest$"
  message: "Pod {{.Name}} uses :latest image tag"
  suggestedFix: "Pin images to a specific version"
```

**Operators:**
- `regex` - field value matches the regular expression
- `equals` - field value exactly equals the value
- `notEquals` - field value does not equal the value

### 2. `fieldNotEmpty` - Ensure a field is populated

Fires when the field is missing, null, or empty.

```yaml
check:
  type: fieldNotEmpty
  resource: pods
  field: ".spec.containers[*].resources.limits"
  message: "Pod {{.Name}} has containers without resource limits"
  suggestedFix: "Set CPU and memory limits on all containers"
```

### 3. `resourceExists` - Verify a cross-reference

Checks that a value referenced by one resource actually exists as another resource.

```yaml
check:
  type: resourceExists
  resource: ingresses
  referenceField: ".spec.tls[*].secretName"
  targetResource: secrets
  targetMatchField: ".metadata.name"
  message: "Ingress {{.Name}} references non-existent TLS secret '{{.RefValue}}'"
  suggestedFix: "Create the TLS secret or update the Ingress"
```

| Field | Description |
|-------|-------------|
| `resource` | The resource that holds the reference |
| `referenceField` | Path to the value being referenced |
| `targetResource` | The resource type that should exist |
| `targetMatchField` | Path on the target to match against (usually `.metadata.name`) |

### 4. `orphanCheck` - Find unreferenced resources

Finds resources that are not referenced by any other resource.

```yaml
check:
  type: orphanCheck
  resource: secrets
  referencedBy: pods
  matchField: ".spec.volumes[*].secret.secretName"
  message: "Secret {{.Name}} is not mounted by any Pod"
  suggestedFix: "Remove the unused Secret or mount it in a Pod"
```

| Field | Description |
|-------|-------------|
| `resource` | The resource to check for orphans |
| `referencedBy` | The resource type that should reference it |
| `matchField` | Path on the referencer that contains the resource name |

### 5. `statusCheck` - Verify a status condition

Checks `.status.conditions[]` for a specific condition type and expected status.

```yaml
check:
  type: statusCheck
  resource: nodes
  condition:
    type: "Ready"
    status: "True"
  message: "Node {{.Name}} is not Ready (status: {{.ConditionStatus}})"
  suggestedFix: "Investigate node health and kubelet logs"
```

Fires when the condition exists but its status does **not** match the expected value.

### 6. `resourceCount` - Count resources and alert above a threshold

Counts all resources of a given type and emits a single finding if the count exceeds the threshold.

```yaml
check:
  type: resourceCount
  resource: endpoints
  threshold: 0             # emit if count > threshold (default 0 = any exist)
  message: "Found {{.Count}} Endpoints resources"
  suggestedFix: "Migrate to EndpointSlice API"
```

| Field | Description |
|-------|-------------|
| `resource` | The resource type to count |
| `threshold` | Emit a finding if count exceeds this value (default `0`, meaning any existence triggers) |

---

## Examples

### Detect Pods without liveness probes

```yaml
id: "USR-WRK001"
name: "Missing Liveness Probe"
description: "Containers without liveness probes won't be restarted on deadlock"
severity: warning
category: workloads
requires:
  - pods
check:
  type: fieldNotEmpty
  resource: pods
  field: ".spec.containers[*].livenessProbe"
  message: "Pod {{.Name}} has containers without a liveness probe"
  suggestedFix: "Add a livenessProbe to each container spec"
```

### Detect Services of type LoadBalancer

```yaml
id: "USR-NET001"
name: "LoadBalancer Service"
description: "LoadBalancer services provision external cloud resources and incur cost"
severity: info
category: networking
requires:
  - services
check:
  type: fieldMatch
  resource: services
  field: ".spec.type"
  operator: equals
  value: "LoadBalancer"
  message: "Service {{.Name}} is type LoadBalancer"
  suggestedFix: "Consider using Ingress or ClusterIP with a shared load balancer"
```

### Detect orphan Secrets not mounted by any Pod

```yaml
id: "USR-CFG001"
name: "Unreferenced Secret"
description: "Secrets not used by any Pod may be stale and a security risk"
severity: info
category: config
requires:
  - secrets
  - pods
check:
  type: orphanCheck
  resource: secrets
  referencedBy: pods
  matchField: ".spec.volumes[*].secret.secretName"
  message: "Secret {{.Name}} is not mounted by any Pod"
  suggestedFix: "Remove the unused Secret or verify it is referenced elsewhere"
```

### Detect Pods running as root

```yaml
id: "USR-SEC001"
name: "Privileged Container"
description: "Containers with privileged mode have full host access"
severity: critical
category: security
requires:
  - pods
check:
  type: fieldMatch
  resource: pods
  field: ".spec.containers[*].securityContext.privileged"
  operator: equals
  value: "true"
  message: "Pod {{.Name}} has a privileged container"
  suggestedFix: "Remove privileged: true unless absolutely required"
```

### Detect too many ConfigMaps in the cluster

```yaml
id: "USR-CFG002"
name: "ConfigMap Sprawl"
description: "Large number of ConfigMaps may indicate stale resources"
severity: info
category: config
requires:
  - configmaps
check:
  type: resourceCount
  resource: configmaps
  threshold: 100
  message: "Found {{.Count}} ConfigMaps - consider cleaning up unused ones"
  suggestedFix: "Audit ConfigMaps and remove those no longer referenced by workloads"
```

### Detect Ingresses without TLS

```yaml
id: "USR-NET002"
name: "Ingress Without TLS"
description: "Ingresses without TLS serve traffic over plain HTTP"
severity: warning
category: networking
requires:
  - ingresses
check:
  type: fieldNotEmpty
  resource: ingresses
  field: ".spec.tls"
  message: "Ingress {{.Name}} has no TLS configuration"
  suggestedFix: "Add a TLS section with a certificate Secret"
```
