/**
 * Kubernetes Resource Type Definitions
 *
 * Focused type definitions for high-value resources.
 * These types cover the properties actually used in action hooks.
 */

// ============================================================================
// Common Types
// ============================================================================

export interface K8sMetadata {
  name: string;
  namespace?: string;
  uid?: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  creationTimestamp?: string;
  deletionTimestamp?: string;
  ownerReferences?: K8sOwnerReference[];
}

export interface K8sOwnerReference {
  apiVersion: string;
  kind: string;
  name: string;
  uid: string;
  controller?: boolean;
}

export interface K8sLabelSelector {
  matchLabels?: Record<string, string>;
  matchExpressions?: Array<{
    key: string;
    operator: string;
    values?: string[];
  }>;
}

// ============================================================================
// Pod Types
// ============================================================================

export interface K8sPod {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: K8sPodSpec;
  status?: K8sPodStatus;
}

export interface K8sPodSpec {
  containers: K8sContainer[];
  initContainers?: K8sContainer[];
  volumes?: K8sVolume[];
  nodeName?: string;
  serviceAccountName?: string;
  restartPolicy?: string;
}

export interface K8sContainer {
  name: string;
  image: string;
  command?: string[];
  args?: string[];
  env?: K8sEnvVar[];
  volumeMounts?: K8sVolumeMount[];
  resources?: K8sResourceRequirements;
}

export interface K8sContainerStatus {
  name: string;
  state?: {
    running?: { startedAt: string };
    waiting?: { reason: string; message?: string };
    terminated?: { exitCode: number; reason?: string; startedAt?: string; finishedAt?: string };
  };
  ready: boolean;
  restartCount: number;
  image: string;
  imageID: string;
}

export interface K8sPodStatus {
  phase?: string;
  conditions?: Array<{ type: string; status: string; reason?: string }>;
  containerStatuses?: K8sContainerStatus[];
  initContainerStatuses?: K8sContainerStatus[];
  podIP?: string;
  hostIP?: string;
  startTime?: string;
}

export interface K8sEnvVar {
  name: string;
  value?: string;
  valueFrom?: {
    configMapKeyRef?: { name: string; key: string };
    secretKeyRef?: { name: string; key: string };
    fieldRef?: { fieldPath: string };
  };
}

export interface K8sVolume {
  name: string;
  configMap?: { name: string };
  secret?: { secretName: string };
  emptyDir?: Record<string, unknown>;
  persistentVolumeClaim?: { claimName: string };
}

export interface K8sVolumeMount {
  name: string;
  mountPath: string;
  readOnly?: boolean;
  subPath?: string;
}

export interface K8sResourceRequirements {
  requests?: { cpu?: string; memory?: string };
  limits?: { cpu?: string; memory?: string };
}

// ============================================================================
// Workload Types (Deployment, StatefulSet, ReplicaSet)
// ============================================================================

export interface K8sDeployment {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: K8sDeploymentSpec;
  status?: K8sDeploymentStatus;
}

export interface K8sDeploymentSpec {
  replicas?: number;
  selector: K8sLabelSelector;
  template: K8sPodTemplateSpec;
  strategy?: {
    type?: string;
    rollingUpdate?: {
      maxUnavailable?: number | string;
      maxSurge?: number | string;
    };
  };
}

export interface K8sDeploymentStatus {
  replicas?: number;
  updatedReplicas?: number;
  readyReplicas?: number;
  availableReplicas?: number;
  conditions?: Array<{ type: string; status: string; reason?: string }>;
}

export interface K8sStatefulSet {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: K8sStatefulSetSpec;
  status?: K8sStatefulSetStatus;
}

export interface K8sStatefulSetSpec {
  replicas?: number;
  selector: K8sLabelSelector;
  template: K8sPodTemplateSpec;
  serviceName: string;
  volumeClaimTemplates?: K8sPersistentVolumeClaim[];
}

export interface K8sStatefulSetStatus {
  replicas?: number;
  readyReplicas?: number;
  currentReplicas?: number;
  updatedReplicas?: number;
  currentRevision?: string;
  updateRevision?: string;
}

export interface K8sReplicaSet {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: K8sReplicaSetSpec;
  status?: K8sReplicaSetStatus;
}

export interface K8sReplicaSetSpec {
  replicas?: number;
  selector: K8sLabelSelector;
  template: K8sPodTemplateSpec;
}

export interface K8sReplicaSetStatus {
  replicas?: number;
  fullyLabeledReplicas?: number;
  readyReplicas?: number;
  availableReplicas?: number;
}

export interface K8sPodTemplateSpec {
  metadata?: Partial<K8sMetadata>;
  spec: K8sPodSpec;
}

// ============================================================================
// Config Types (Secret, ConfigMap)
// ============================================================================

export interface K8sSecret {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  type?: string;
  data?: Record<string, string>;
  stringData?: Record<string, string>;
}

export interface K8sConfigMap {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  data?: Record<string, string>;
  binaryData?: Record<string, string>;
}

// ============================================================================
// Network Types (Service, Ingress)
// ============================================================================

export interface K8sService {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: K8sServiceSpec;
  status?: K8sServiceStatus;
}

export interface K8sServiceSpec {
  type?: string;
  selector?: Record<string, string>;
  ports?: K8sServicePort[];
  clusterIP?: string;
  externalIPs?: string[];
  loadBalancerIP?: string;
}

export interface K8sServicePort {
  name?: string;
  protocol?: string;
  port: number;
  targetPort?: number | string;
  nodePort?: number;
}

export interface K8sServiceStatus {
  loadBalancer?: {
    ingress?: Array<{ ip?: string; hostname?: string }>;
  };
}

export interface K8sIngress {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: K8sIngressSpec;
  status?: K8sIngressStatus;
}

export interface K8sIngressSpec {
  ingressClassName?: string;
  rules?: K8sIngressRule[];
  tls?: Array<{
    hosts?: string[];
    secretName?: string;
  }>;
  defaultBackend?: K8sIngressBackend;
}

export interface K8sIngressRule {
  host?: string;
  http?: {
    paths: Array<{
      path?: string;
      pathType?: string;
      backend: K8sIngressBackend;
    }>;
  };
}

export interface K8sIngressBackend {
  service?: {
    name: string;
    port: {
      number?: number;
      name?: string;
    };
  };
}

export interface K8sIngressStatus {
  loadBalancer?: {
    ingress?: Array<{ ip?: string; hostname?: string }>;
  };
}

// ============================================================================
// Cluster Types (Namespace)
// ============================================================================

export interface K8sNamespace {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec?: K8sNamespaceSpec;
  status?: K8sNamespaceStatus;
}

export interface K8sNamespaceSpec {
  finalizers?: string[];
}

export interface K8sNamespaceStatus {
  phase?: string;
}

// ============================================================================
// Storage Types
// ============================================================================

export interface K8sPersistentVolumeClaim {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: K8sPVCSpec;
  status?: K8sPVCStatus;
}

export interface K8sPVCSpec {
  accessModes?: string[];
  resources?: {
    requests?: { storage?: string };
  };
  storageClassName?: string;
  volumeName?: string;
}

export interface K8sPVCStatus {
  phase?: string;
  capacity?: { storage?: string };
}

// ============================================================================
// Additional Workload Types
// ============================================================================

export interface K8sDaemonSet {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: K8sDaemonSetSpec;
  status?: K8sDaemonSetStatus;
}

export interface K8sDaemonSetSpec {
  selector: K8sLabelSelector;
  template: K8sPodTemplateSpec;
  updateStrategy?: {
    type?: string;
    rollingUpdate?: { maxUnavailable?: number | string };
  };
}

export interface K8sDaemonSetStatus {
  currentNumberScheduled?: number;
  desiredNumberScheduled?: number;
  numberReady?: number;
  updatedNumberScheduled?: number;
  numberAvailable?: number;
}

export interface K8sJob {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: K8sJobSpec;
  status?: K8sJobStatus;
}

export interface K8sJobSpec {
  template: K8sPodTemplateSpec;
  completions?: number;
  parallelism?: number;
  backoffLimit?: number;
}

export interface K8sJobStatus {
  active?: number;
  succeeded?: number;
  failed?: number;
  completionTime?: string;
  startTime?: string;
}

export interface K8sCronJob {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: K8sCronJobSpec;
  status?: K8sCronJobStatus;
}

export interface K8sCronJobSpec {
  schedule: string;
  jobTemplate: { spec: K8sJobSpec };
  suspend?: boolean;
  successfulJobsHistoryLimit?: number;
  failedJobsHistoryLimit?: number;
}

export interface K8sCronJobStatus {
  active?: Array<{ name: string; namespace?: string }>;
  lastScheduleTime?: string;
}

// ============================================================================
// Additional Network Types
// ============================================================================

export interface K8sEndpoints {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  subsets?: Array<{
    addresses?: Array<{ ip: string; hostname?: string }>;
    ports?: Array<{ name?: string; port: number; protocol?: string }>;
  }>;
}

export interface K8sEndpointSlice {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  addressType: string;
  endpoints?: Array<{
    addresses: string[];
    conditions?: { ready?: boolean };
  }>;
  ports?: Array<{ name?: string; port?: number; protocol?: string }>;
}

export interface K8sNetworkPolicy {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: K8sNetworkPolicySpec;
}

export interface K8sNetworkPolicySpec {
  podSelector: K8sLabelSelector;
  policyTypes?: string[];
  ingress?: Array<{
    from?: Array<{ podSelector?: K8sLabelSelector; namespaceSelector?: K8sLabelSelector }>;
    ports?: Array<{ protocol?: string; port?: number | string }>;
  }>;
  egress?: Array<{
    to?: Array<{ podSelector?: K8sLabelSelector; namespaceSelector?: K8sLabelSelector }>;
    ports?: Array<{ protocol?: string; port?: number | string }>;
  }>;
}

export interface K8sIngressClass {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec?: {
    controller: string;
    parameters?: { apiGroup?: string; kind: string; name: string };
  };
}

// ============================================================================
// Additional Config Types
// ============================================================================

export interface K8sHorizontalPodAutoscaler {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: K8sHPASpec;
  status?: K8sHPAStatus;
}

export interface K8sHPASpec {
  scaleTargetRef: { apiVersion: string; kind: string; name: string };
  minReplicas?: number;
  maxReplicas: number;
  metrics?: Array<{
    type: string;
    resource?: { name: string; target: { type: string; averageUtilization?: number } };
  }>;
}

export interface K8sHPAStatus {
  currentReplicas?: number;
  desiredReplicas?: number;
  currentMetrics?: Array<{
    type: string;
    resource?: { name: string; current: { averageUtilization?: number; averageValue?: string } };
  }>;
}

export interface K8sPodDisruptionBudget {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: K8sPDBSpec;
  status?: K8sPDBStatus;
}

export interface K8sPDBSpec {
  selector?: K8sLabelSelector;
  minAvailable?: number | string;
  maxUnavailable?: number | string;
}

export interface K8sPDBStatus {
  currentHealthy?: number;
  desiredHealthy?: number;
  disruptionsAllowed?: number;
  expectedPods?: number;
}

export interface K8sResourceQuota {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec?: {
    hard?: Record<string, string>;
    scopeSelector?: { matchExpressions?: Array<{ scopeName: string; operator: string; values?: string[] }> };
  };
  status?: {
    hard?: Record<string, string>;
    used?: Record<string, string>;
  };
}

export interface K8sLease {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec?: {
    holderIdentity?: string;
    leaseDurationSeconds?: number;
    acquireTime?: string;
    renewTime?: string;
  };
}

export interface K8sLimitRange {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec?: {
    limits?: Array<{
      type?: string;
      max?: Record<string, string>;
      min?: Record<string, string>;
      default?: Record<string, string>;
      defaultRequest?: Record<string, string>;
    }>;
  };
}

// ============================================================================
// Additional Storage Types
// ============================================================================

export interface K8sPersistentVolume {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: K8sPVSpec;
  status?: K8sPVStatus;
}

export interface K8sPVSpec {
  capacity?: { storage?: string };
  accessModes?: string[];
  persistentVolumeReclaimPolicy?: string;
  storageClassName?: string;
  claimRef?: { name: string; namespace: string };
}

export interface K8sPVStatus {
  phase?: string;
}

export interface K8sStorageClass {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  provisioner: string;
  parameters?: Record<string, string>;
  reclaimPolicy?: string;
  volumeBindingMode?: string;
  allowVolumeExpansion?: boolean;
}

export interface K8sCSIDriver {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: {
    attachRequired?: boolean;
    podInfoOnMount?: boolean;
    volumeLifecycleModes?: string[];
  };
}

export interface K8sCSINode {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: {
    drivers: Array<{
      name: string;
      nodeID: string;
      topologyKeys?: string[];
    }>;
  };
}

// ============================================================================
// Additional Cluster Types
// ============================================================================

export interface K8sNode {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec?: {
    podCIDR?: string;
    podCIDRs?: string[];
    taints?: Array<{ key: string; value?: string; effect: string }>;
    unschedulable?: boolean;
  };
  status?: {
    capacity?: Record<string, string>;
    allocatable?: Record<string, string>;
    conditions?: Array<{ type: string; status: string; reason?: string }>;
    addresses?: Array<{ type: string; address: string }>;
    nodeInfo?: {
      osImage?: string;
      kernelVersion?: string;
      kubeletVersion?: string;
    };
  };
}

export interface K8sEvent {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  involvedObject?: {
    kind: string;
    name: string;
    namespace?: string;
    uid?: string;
  };
  reason?: string;
  message?: string;
  type?: string;
  count?: number;
  firstTimestamp?: string;
  lastTimestamp?: string;
}

export interface K8sPriorityClass {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  value: number;
  globalDefault?: boolean;
  description?: string;
}

export interface K8sMutatingWebhookConfiguration {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  webhooks?: Array<{
    name: string;
    clientConfig: { service?: { name: string; namespace: string; path?: string }; url?: string };
    rules?: Array<{ operations: string[]; apiGroups: string[]; apiVersions: string[]; resources: string[] }>;
  }>;
}

export interface K8sValidatingWebhookConfiguration {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  webhooks?: Array<{
    name: string;
    clientConfig: { service?: { name: string; namespace: string; path?: string }; url?: string };
    rules?: Array<{ operations: string[]; apiGroups: string[]; apiVersions: string[]; resources: string[] }>;
  }>;
}

// ============================================================================
// RBAC Types
// ============================================================================

export interface K8sRole {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  rules?: Array<{
    apiGroups: string[];
    resources: string[];
    verbs: string[];
    resourceNames?: string[];
  }>;
}

export interface K8sClusterRole {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  rules?: Array<{
    apiGroups: string[];
    resources: string[];
    verbs: string[];
    resourceNames?: string[];
  }>;
  aggregationRule?: {
    clusterRoleSelectors?: Array<{ matchLabels?: Record<string, string> }>;
  };
}

export interface K8sRoleBinding {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  subjects?: Array<{
    kind: string;
    name: string;
    namespace?: string;
  }>;
  roleRef: {
    apiGroup: string;
    kind: string;
    name: string;
  };
}

export interface K8sClusterRoleBinding {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  subjects?: Array<{
    kind: string;
    name: string;
    namespace?: string;
  }>;
  roleRef: {
    apiGroup: string;
    kind: string;
    name: string;
  };
}

export interface K8sServiceAccount {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  secrets?: Array<{ name: string }>;
  imagePullSecrets?: Array<{ name: string }>;
  automountServiceAccountToken?: boolean;
}

// ============================================================================
// Custom Resources
// ============================================================================

export interface K8sCustomResourceDefinition {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec: {
    group: string;
    names: {
      plural: string;
      singular: string;
      kind: string;
      shortNames?: string[];
    };
    scope: string;
    versions: Array<{
      name: string;
      served: boolean;
      storage: boolean;
      schema?: unknown;
    }>;
  };
  status?: {
    conditions?: Array<{ type: string; status: string; reason?: string }>;
    acceptedNames?: { plural: string; kind: string };
  };
}

// ============================================================================
// Helm Types
// ============================================================================

export interface K8sHelmRelease {
  name: string;
  namespace: string;
  revision: number;
  status: string;
  chart: string;
  appVersion: string;
  updated: string;
}

// ============================================================================
// Generic Resource Type (for base actions)
// ============================================================================

export interface K8sResource {
  apiVersion?: string;
  kind?: string;
  metadata: K8sMetadata;
  spec?: unknown;
  status?: unknown;
}
