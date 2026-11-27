// Golden-parity generator. It runs the REAL legacy list code (sortItems, createFilter,
// compileColumnFilters, computeColumnUniqueValues, detectColumnFilterTypes) over fixtures and
// records the results in pkg/rows/testdata/golden/<type>.json. The Go row engine must
// reproduce every recorded result.
//
//   GOLDEN=1 npx vitest run src/test/golden   rewrites the files
//   npx vitest run src/test/golden            asserts the committed files still equal the generator output
//
// Search queries and filters live here so both sides test exactly the same cases.

import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import {
    compareNames,
    compileColumnFilters,
    computeColumnUniqueValues,
    detectColumnFilterTypes,
    getColumnValue,
    getNumericValue,
    sortItems,
} from '~/components/shared/resourceListPipeline';
import { createFilter } from '~/utils/search';
import { semanticColumns } from '~/utils/columnSemantics';
import { namespaceSemantics } from '~/features/cluster/namespaces/semantics';
import { configMapSemantics } from '~/features/config/configmaps/semantics';
import { secretSemantics } from '~/features/config/secrets/semantics';
import { eventSemantics } from '~/features/cluster/events/semantics';
import { priorityClassSemantics } from '~/features/cluster/priorityclasses/semantics';
import { ingressClassSemantics } from '~/features/network/ingressclasses/semantics';
import { storageClassSemantics } from '~/features/storage/storageclass/semantics';
import { validatingWebhookSemantics, mutatingWebhookSemantics } from '~/features/cluster/webhooks/semantics';
import { csiDriverSemantics } from '~/features/storage/csidrivers/semantics';
import { csiNodeSemantics } from '~/features/storage/csinodes/semantics';
import { pvSemantics } from '~/features/storage/pv/semantics';
import { clusterRoleSemantics } from '~/features/access-control/clusterroles/semantics';
import { clusterRoleBindingSemantics } from '~/features/access-control/clusterrolebindings/semantics';
import { hpaSemantics } from '~/features/config/hpas/semantics';
import { pdbSemantics } from '~/features/config/pdbs/semantics';
import { resourceQuotaSemantics } from '~/features/config/resourcequotas/semantics';
import { limitRangeSemantics } from '~/features/config/limitranges/semantics';
import { leaseSemantics } from '~/features/config/leases/semantics';
import { networkPolicySemantics } from '~/features/network/networkpolicies/semantics';
import { serviceAccountSemantics } from '~/features/access-control/serviceaccounts/semantics';
import { endpointsSemantics } from '~/features/network/endpoints/semantics';
import { endpointSliceSemantics } from '~/features/network/endpointslices/semantics';
import { serviceSemantics } from '~/features/network/services/semantics';
import { ingressSemantics } from '~/features/network/ingresses/semantics';
import { roleSemantics } from '~/features/access-control/roles/semantics';
import { roleBindingSemantics } from '~/features/access-control/rolebindings/semantics';
import { replicaSetSemantics } from '~/features/workloads/replicasets/semantics';
import { jobSemantics } from '~/features/workloads/jobs/semantics';
import { daemonSetSemantics } from '~/features/workloads/daemonsets/semantics';
import { createDeploymentSemantics, deploymentSemantics } from '~/features/workloads/deployments/semantics';
import { createStatefulSetSemantics, statefulSetSemantics } from '~/features/workloads/statefulsets/semantics';
import { deploymentPodMaps } from '~/features/workloads/podDerivation';
import { getDeploymentPods } from '~/utils/k8s-helpers';
import { pvcSemantics } from '~/features/storage/pvc/semantics';
import { createPodSemantics, podMetricKey, podSemantics } from '~/features/workloads/pods/semantics';
import { createNodeSemantics, nodeSemantics } from '~/features/cluster/nodes/semantics';
import { createNamespaceSemantics } from '~/features/cluster/namespaces/semantics';
import {
    NAMESPACE_RECORD_FIELDS, NODE_RECORD_FIELDS, POD_RECORD_FIELDS, aggregateNamespaceMetrics, metricsRecordText, recordOf,
    transformNodeMetrics, transformPodMetrics,
} from '~/utils/metricsTransform';
import {
    GOLDEN_NOW,
    configMapFixtures,
    eventFixtures,
    namespaceFixtures,
    priorityClassFixtures,
    ingressClassFixtures,
    storageClassFixtures,
    validatingWebhookFixtures,
    mutatingWebhookFixtures,
    csiDriverFixtures,
    csiNodeFixtures,
    pvFixtures,
    clusterRoleFixtures,
    clusterRoleBindingFixtures,
    hpaFixtures,
    pdbFixtures,
    resourceQuotaFixtures,
    limitRangeFixtures,
    leaseFixtures,
    networkPolicyFixtures,
    serviceAccountFixtures,
    roleFixtures,
    endpointsFixtures,
    ingressFixtures,
    serviceFixtures,
    endpointSliceFixtures,
    roleBindingFixtures,
    secretFixtures,
    replicaSetFixtures,
    jobFixtures,
    daemonSetFixtures,
    deploymentFixtures,
    statefulSetFixtures,
    workloadPodStates,
    pvcFixtures,
    podFixtures,
    nodeFixtures,
    podOverlayStates,
    nodeOverlayStates,
    namespaceOverlayStates,
    type FixtureSet,
    type OverlayRaw,
    type NamespaceOverlayRaw,
} from './fixtures';
import { readGolden, regenerate, writeGolden } from './goldenIo';

const COMMON_QUERIES = [
    'alp', 'ALP', 'beta', 'eta', 'kube', 'x', '', '   ', 'zzzz-no-match', '10', 'caf', 'CAF\u00c9', 'emoji',
    'name:alp', 'name:"beta"', "name:'item'", 'n:/^a/', 'name:/^A/i', 'name:/alpha|zeta/', 'name:/ALPHA/', '/ALPHA/i', '/^beta-\\d+$/',
    '/(/', 'name:/(/', 'name:/[/i', '/(?<=a)l/', 'name:/^(?!alpha).*a$/', 'name:/a.b/s', 'name:/^b/y', 'name:/a/gg', 'name:/a/x',
    'name:alpha OR name:zeta', 'alpha OR zeta OR x', 'alpha or zeta', 'name:/^a/ OR name:/^b/ kube', 'alpha  OR  beta', ' OR alpha', 'alpha OR ',
    'foo:bar', 'foo:bar OR alpha', 'uid:uid-0003', 'uid:edge', 'label:app=web', 'l:app=', 'l:10=ten', 'labels:tier', 'annotation:note', 'a:a/b',
    'ns:prod', 'namespace:Prod', 'ns:/^team/', 'ns:team-a name:same', 'NS:default', 'status:active', 'phase:terminating', 'state:weird', 'finalizer:kube',
    'type:opaque', 'type:/^kubernetes\\.io/', 'secrettype:tls', 'keycount:3', 'count:2', 'datakeys:0', 'keys:config', 'key:10', 'binary:blob', 'binarykey:5',
    'value:1000', 'priority:7', 'default:true', 'isdefault:false', 'preemption:never', 'desc:critical', 'description:batch', 'globaldefault:true',
    'reason:backoff', 'r:/^Fail/', 'message:probe', 'msg:/503/', 'm:"restarting failed"', 'object:pod', 'obj:/^Node\\//', 'involved:deployment', 'objectkind:pod', 'ok:node',
    'objectname:alpha', 'on:beta', 'source:kubelet', 'src:/\\/node-1$/', 'c:1', 'kind:warning', 't:normal',
    'name:"unterminated', "name:'unterminated", 'name:,', 'name:/', 'a:b:c', 'http://x', 'a-b:c', 'x:y z', '/abc', 'abc/', 'name:/a/ , beta',
    'name:\u00e9', 'name:\u00c9clair', 'name:/\u00e9/i', 'name:/^\\p{L}/u', 'name:/\\w+-\\d/', 'name:/\\bbeta\\b/',
];

// Queries aimed at the search fields of one type, on top of the shared list.
const TYPE_QUERIES: Record<string, string[]> = {
    validatingwebhookconfigurations: [
        'webhooks:2', 'count:3', 'webhookcount:0', 'failurepolicy:ignore', 'failure:fail', 'onfailure:/^Ignore/', 'sideeffects:none', 'sideeffect:some',
        'service:ns/svc', 'services:/^cert-manager/', 'target:default', 'resources:pods', 'resource:/^jobs/', 'operations:create', 'ops:delete', 'operation:*',
        'reinvocation:never', 'failurepolicy:"Ignore Fail"', 'service:ns/svc OR resources:deployments',
    ],
    mutatingwebhookconfigurations: [
        'webhooks:2', 'count:3', 'failurepolicy:ignore', 'sideeffects:none', 'service:ns/svc', 'target:default', 'resources:pods', 'operations:create',
        'reinvocation:never', 'reinvocationpolicy:ifneeded', 'reinvocation:/^IfNeeded Never$/', 'reinvocation:"Never IfNeeded"',
    ],
    csidrivers: [
        'attachrequired:true', 'attach:false', 'attachreq:true', 'podinfoonmount:true', 'podinfo:false', 'storagecapacity:true', 'capacity:false',
        'volumemodes:persistent', 'modes:ephemeral', 'volumelifecyclemodes:/^Persistent,Ephemeral$/', 'fsgrouppolicy:file', 'fsgroup:none', 'fspolicy:/Type$/',
    ],
    csinodes: [
        'drivercount:2', 'drivers:0', 'numDrivers:1', 'numdrivers:1', 'drivernames:ebs', 'driver:/^one,two/', 'nodeids:n1', 'nodeid:/^node-0/', 'driver:csi OR nodeid:n3',
    ],
    persistentvolumes: [
        'name:standard', 'uid:uid-edge-1', 'label:tier=x', 'capacity:1Gi', 'status:bound', 'claim:default', 'storageclass:gp3',
    ],
    clusterroles: [
        'rules:pods', 'rule:/^apps\\//', 'rulecount:2', 'rulescount:0', 'numrules:3', 'verbs:get', 'verb:*', 'resources:secrets', 'resource:/^jobs/', 'apigroups:core', 'group:batch', 'groups:apps',
        'aggregation:a=1', 'aggregate:true', 'aggregationrule:/^a=1,b=2/', 'aggregation:c=3', 'verbs:"get list"', 'rules:/,get$/',
    ],
    clusterrolebindings: [
        'roleref:clusterrole/admin', 'role:view', 'ref:/^Role\\//', 'rolerefkind:role', 'refkind:ClusterRole', 'rolerefname:admin', 'refname:view', 'rolename:/^Z/',
        'subjects:user/alice', 'subject:/^Group/', 'subjectcount:2', 'subjectscount:0', 'numsubjects:1', 'subjectkind:serviceaccount', 'subjectkinds:"User Group"',
        'subjectnames:alice', 'subjectname:/sa/', 'users:bob', 'user:sa', 'groups:devs', 'group:g1', 'serviceaccounts:a/sa', 'serviceaccount:default/', 'sa:/^sb$/',
    ],
    hpas: [
        'target:Deployment/web', 'reference:statefulset', 'ref:/^Deployment\\//', 'scaletarget:rs-1', 'targetkind:deployment', 'kind:Rollout', 'refkind:/^R/',
        'targetname:web', 'refname:/^z/i', 'minreplicas:1', 'min:2', 'min:0', 'maxreplicas:10', 'max:100', 'max:0', 'currentreplicas:3', 'current:1', 'replicas:5',
        'desiredreplicas:0', 'desired:2', 'metrics:resource', 'metrictype:pods', 'metrics:"Resource Pods"', 'metrics:/external/i', 'target:/\\/web$/ OR min:5',
    ],
    pdbs: [
        'minavailable:2', 'min:50%', 'min:0', 'maxunavailable:1', 'max:25%', 'selector:app=web', 'podselector:/tier=data/', 'selector:10=ten', 'currenthealthy:3',
        'healthy:0', 'current:2', 'desiredhealthy:2', 'desired:1', 'disruptionsallowed:0', 'allowed:1', 'disruptions:2', 'expectedpods:3', 'expected:0', 'pods:5',
        'min:/^[0-9]+$/', 'selector:"a=1 b=2"',
    ],
    resourcequotas: [
        'hard:cpu=2', 'limits:memory=4Gi', 'resources:pods=10', 'hard:/requests\\.cpu=1/', 'used:cpu=500m', 'usage:pods', 'scopes:BestEffort', 'scope:/^Not/',
        'resourcecount:3', 'count:0', 'count:1', 'hard:"cpu=2 memory=4Gi"', 'scopes:"BestEffort NotTerminating PriorityClass"',
    ],
    limitranges: [
        'limittype:Container', 'type:pod', 'types:persistentvolumeclaim', 'limitcount:2', 'count:0', 'limits:3', 'hascontainer:true', 'container:false', 'haspod:true',
        'pod:false', 'haspvc:true', 'pvc:false', 'limittype:"Container Pod"', 'limittype:/^Container Container$/',
    ],
    leases: [
        'holder:node-1', 'holderidentity:/^kube-controller/', 'leader:zeta', 'owner:10', 'holder:caf', 'duration:15', 'leaseduration:30', 'leasedurationseconds:0',
        'transitions:0', 'leasetransitions:5', 'changes:12', 'holder:-', 'holder:/^$/',
    ],
    networkpolicies: [
        'podselector:all', 'selector:app=web', 'pods:/tier=/', 'policytype:ingress', 'policytypes:"Ingress Egress"', 'type:egress', 'types:/^Ingress Egress$/',
        'ingress:true', 'hasingress:false', 'egress:true', 'hasegress:true', 'ingresscount:2', 'ingressrules:0', 'egresscount:3', 'egressrules:1', 'selector:10=ten',
    ],
    endpoints: [
        'subsets:2', 'subsetcount:0', 'subsets:1', 'addresses:10.9.9.1', 'ips:/^10\\.9\\.9\\./', 'ip:10.0', 'ready:2', 'readycount:0', 'notready:2', 'notreadycount:1',
        'ports:80/TCP', 'port:8080/tcp', 'ports:"80/TCP 81/TCP"', 'ports:/53\\/UDP/', 'targetref:pod/p', 'target:/^Node\\//', 'targets:pod/web-1', 'ready:0 notready:0',
    ],
    endpointslices: [
        'name:web', 'label:kubernetes.io/service-name=web', 'labels:service-name', 'l:10=ten', 'addresstype:ipv4', 'endpoints:1', 'ports:80', 'service:web',
    ],
    services: [
        'type:loadbalancer', 'servicetype:nodeport', 'type:clusterip', 'type:externalname', 'clusterip:10.96', 'ip:none', 'cluster-ip:/^10\\.0\\./', 'externalip:203.0.113.7', 'extip:9.9.9.9', 'external-ip:/8\\.8/',
        'loadbalancerip:1.1.1.1', 'lbip:elb.example.com', 'loadbalancer:/^h1/', 'lbip:"1.1.1.1 2.2.2.2"', 'port:80', 'ports:"80 443"', 'port:/^5/', 'targetport:8080', 'targetports:https', 'targetport:named',
        'nodeport:30080', 'nodeports:31000', 'nodeport:0', 'protocol:tcp', 'protocols:"TCP UDP"', 'protocol:udp', 'selector:app=web', 'sel:/tier=/', 'selector:10=ten', 'selector:"1=one a=1"',
        'sessionaffinity:clientip', 'affinity:none', 'session:/^None$/', 'externalname:db.example.com', 'extname:/ext/', 'headless:true', 'headless:false',
    ],
    ingresses: [
        'name:empty', 'label:tier=x', 'annotation:kubernetes.io/ingress.class=nginx', 'a:ingress.class', 'host:app.example.com', 'class:nginx', 'status:active', 'rules:2',
    ],
    roles: [
        'rules:pods', 'rule:/^apps\\//', 'rulecount:2', 'rulescount:0', 'numrules:3', 'verbs:get', 'verb:*', 'resources:secrets', 'resource:/^jobs/', 'apigroups:core', 'group:batch', 'groups:apps',
        'verbs:"get list"', 'rules:/,get$/', 'aggregation:a=1',
    ],
    rolebindings: [
        'roleref:role/admin', 'role:view', 'ref:/^ClusterRole\\//', 'rolerefkind:role', 'refkind:ClusterRole', 'rolerefname:admin', 'refname:view', 'rolename:/^Z/',
        'subjects:user/alice', 'subject:/^Group/', 'subjectcount:2', 'subjectscount:0', 'numsubjects:1', 'subjectkind:serviceaccount', 'subjectkinds:"User Group"',
        'subjectnames:alice', 'subjectname:/sa/', 'users:bob', 'user:sa', 'groups:devs', 'group:g1', 'serviceaccounts:a/sa', 'serviceaccount:default/', 'sa:/^sb$/',
    ],
    replicasets: [
        'image:nginx', 'images:envoy', 'image:busybox', 'image:/^registry\\.example\\.com\\//', 'container:sidecar', 'containers:app', 'container:init', 'serviceaccount:builder', 'sa:/^Zeta/i', 'serviceaccount:legacy-sa',
        'selector:app=web', 'sel:/tier=/', 'selector:10=ten', 'selector:"1=one a=1"', 'controlledby:deployment', 'controller:Rollout', 'owner:/^Cron/', 'controlledby:ConfigMap', 'controllername:web', 'ownername:/^c/', 'controllername:\u00e9clair',
        'replicas:3', 'desired:0', 'current:1', 'currentreplicas:3', 'ready:0', 'readyreplicas:2', 'available:2', 'availablereplicas:0', 'fulllabeled:3', 'labeled:0', 'image:busybox OR container:init',
    ],
    jobs: [
        'completions:1', 'completions:10', 'parallelism:1', 'parallelism:5', 'succeeded:3', 'success:0', 'failed:4', 'failures:0', 'active:2', 'running:1',
        'condition:complete', 'status:failed', 'state:/^Susp/', 'condition:', 'backofflimit:6', 'backoff:3', 'backoff:0',
        'image:nginx', 'images:envoy', 'image:busybox', 'container:sidecar', 'containers:init', 'serviceaccount:job-sa', 'sa:legacy-sa', 'sa:old-sa',
        'controlledby:CronJob', 'controller:/^Dep/', 'owner:configmap', 'controllername:nightly', 'ownername:web', 'controllername:\u00e9clair',
    ],
    daemonsets: [
        'desired:3', 'desirednumber:0', 'current:5', 'currentnumber:1', 'ready:3', 'readynumber:0', 'available:4', 'availablenumber:0', 'updated:12', 'updatednumber:0', 'misscheduled:1', 'misscheduled:0',
        'updatestrategy:ondelete', 'strategy:/^Rolling/', 'strategy:rollingupdate',
        'image:nginx', 'images:envoy', 'container:sidecar', 'containers:init', 'serviceaccount:ds-sa', 'sa:legacy-sa', 'sa:old-sa', 'sa:/^Zeta/i',
        'selector:app=web', 'sel:/tier=/', 'selector:10=ten', 'selector:"1=one a=1"', 'controlledby:daemonset', 'controllername:x',
    ],
    deployments: [
        'replicas:3', 'desired:0', 'replicas:10', 'ready:0', 'readyreplicas:1', 'available:3', 'availablereplicas:0', 'updated:3', 'updatedreplicas:0',
        'strategy:recreate', 'updatestrategy:/^Rolling/', 'strategy:', 'paused:true', 'paused:false',
        'image:nginx', 'images:envoy', 'container:sidecar', 'containers:init', 'serviceaccount:dep-sa', 'sa:legacy-sa', 'sa:old-sa',
        'selector:app=web', 'sel:/tier=/', 'selector:10=ten', 'selector:"1=one a=1"', 'controlledby:replicaset', 'controllername:x',
    ],
    statefulsets: [
        'replicas:3', 'desired:0', 'replicas:5', 'ready:0', 'readyreplicas:2', 'current:3', 'currentreplicas:0', 'updated:1', 'updatedreplicas:0',
        'servicename:web', 'service:/^db/', 'headless:svc', 'servicename:', 'podmanagementpolicy:parallel', 'podpolicy:orderedready', 'podpolicy:/^P/',
        'updatestrategy:ondelete', 'strategy:/^Rolling/', 'strategy:rollingupdate',
        'image:nginx', 'images:envoy', 'container:sidecar', 'containers:init', 'serviceaccount:sts-sa', 'sa:legacy-sa', 'sa:old-sa',
        'selector:app=web', 'sel:/tier=/', 'selector:10=ten', 'controlledby:statefulset', 'controllername:x',
    ],
    pods: [
        'nodename:node-a', 'node:/^ip-10/', 'node:Node-Z', 'status:running', 'phase:pending', 's:crashloopbackoff', 'status:ImagePullBackOff', 'status:ErrImagePull', 'status:Init:Error', 'status:init:running', 'status:terminating', 'status:Unknown', 'status:weird', 'status:/^Init/',
        'ip:10.1', 'podip:/^10\\.1\\.0\\./', 'hostip:192.168', 'hostip:/\\.9$/', 'restarts:0', 'restart:12', 'restarts:5', 'restarts:/^1/',
        'controlledby:deployment', 'controller:Rollout', 'owner:/^Cron/', 'controllername:web', 'ownername:/^c/', 'controllername:\u00e9clair',
        'container:sidecar', 'containers:app', 'container:init', 'image:nginx', 'images:envoy', 'image:busybox', 'image:/^registry\\.example\\.com\\//',
        'serviceaccount:builder', 'sa:/^Zeta/i', 'sa:legacy-sa', 'sa:new-sa', 'qos:guaranteed', 'qosclass:besteffort', 'qos:/^B/', 'status:running node:node-a', 'status:running OR status:failed',
    ],
    nodes: [
        'status:ready', 'state:notready', 'status:/^Not/', 'role:control-plane', 'roles:worker', 'role:etcd', 'role:/^m/', 'role:10', 'version:v1.29', 'kubeletversion:/^v1\\.3/', 'k8sversion:v1.9.0',
        'os:ubuntu', 'osimage:flatcar', 'kernel:6.1', 'kernelversion:5.15', 'containerruntime:cri-o', 'runtime:containerd', 'cri:/^containerd/', 'arch:arm64', 'architecture:amd64',
        'podcidr:10.244', 'cidr:/\\/24$/', 'internalip:10.0.0', 'ip:10.0.1', 'externalip:34.1', 'extip:/^35/', 'hostname:h', 'host:x',
        'memorypressure:true', 'memory:false', 'diskpressure:true', 'disk:false', 'pidpressure:true', 'pid:false', 'unschedulable:true', 'cordon:false', 'cordoned:true',
        'taints:dedicated=gpu:NoSchedule', 'taint:/^k=:/', 'taints:NoExecute', 'taints:"a=b:PreferNoSchedule"', 'role:control-plane OR role:worker',
    ],
    serviceaccounts: [
        'secrets:sa-token', 'secret:/^a$/', 'secretcount:2', 'secretscount:0', 'numsecrets:5', 'imagepullsecrets:regcred', 'pullsecrets:r1', 'imagepull:/dup/',
        'automount:true', 'automounttoken:false', 'automountserviceaccounttoken:TRUE',
    ],
};

const NAMESPACE_REGEX_TYPES = new Set(['pods', 'hpas', 'pdbs', 'resourcequotas', 'limitranges', 'leases', 'networkpolicies', 'serviceaccounts', 'roles', 'rolebindings', 'endpoints', 'endpointslices', 'services', 'ingresses', 'replicasets', 'jobs', 'daemonsets', 'persistentvolumeclaims', 'deployments', 'statefulsets']);

type FilterCase = { name: string; filters: Record<string, any> };

function filterCases(type: string, columns: string[]): FilterCase[] {
    const has = (c: string) => columns.includes(c);
    const cases: FilterCase[] = [];
    cases.push({ name: 'name regex', filters: { name: { type: 'regex', conditions: ['^a'] } } });
    cases.push({ name: 'name regex or', filters: { name: { type: 'regex', conditions: ['^a', 'zeta'], logic: 'or' } } });
    cases.push({ name: 'name regex and', filters: { name: { type: 'regex', conditions: ['a', 'e'], logic: 'and' } } });
    cases.push({ name: 'name regex invalid ignored', filters: { name: { type: 'regex', conditions: ['('] } } });
    cases.push({ name: 'name regex invalid or valid', filters: { name: { type: 'regex', conditions: ['(', 'zeta'], logic: 'or' } } });
    cases.push({ name: 'name regex empty conditions', filters: { name: { type: 'regex', conditions: [] } } });
    cases.push({ name: 'name regex empty conditions or', filters: { name: { type: 'regex', conditions: [], logic: 'or' } } });
    cases.push({ name: 'name regex pattern field', filters: { name: { type: 'regex', pattern: 'ALPHA' } } });
    cases.push({ name: 'name regex lookahead', filters: { name: { type: 'regex', conditions: ['^(?=.*a)b'] } } });
    cases.push({ name: 'name regex unicode', filters: { name: { type: 'regex', conditions: ['\u00e9'] } } });
    cases.push({ name: 'unknown column ignored', filters: { nope: { type: 'select', values: ['x'] } } });
    if (has('age')) {
        cases.push({ name: 'age older than a day', filters: { age: { type: 'numeric', conditions: [{ operator: '>', value: 24 }] } } });
        cases.push({ name: 'age between', filters: { age: { type: 'numeric', conditions: [{ operator: '>=', value: 2 }, { operator: '<', value: 30 }], logic: 'and' } } });
        cases.push({ name: 'age either', filters: { age: { type: 'numeric', conditions: [{ operator: '<', value: 1 }, { operator: '>', value: 60 }], logic: 'or' } } });
        cases.push({ name: 'age unknown operator', filters: { age: { type: 'numeric', conditions: [{ operator: '~', value: 1 }] } } });
        cases.push({ name: 'age single operator fields', filters: { age: { type: 'numeric', operator: '<=', value: 12 } } });
        cases.push({ name: 'age equals NaN never', filters: { age: { type: 'numeric', conditions: [{ operator: '=', value: Number.NaN }] } } });
        cases.push({ name: 'age not equals', filters: { age: { type: 'numeric', conditions: [{ operator: '!=', value: 0 }] } } });
    }
    const selects: Record<string, string[][]> = {
        namespaces: [['status', ['Active']], ['status', ['Active', 'Weird']], ['status', []]] as any,
        configmaps: [['namespace', ['default', 'prod']], ['keys', ['config', '']]] as any,
        secrets: [['type', ['Opaque']], ['type', ['kubernetes.io/tls', 'helm.sh/release.v1']], ['namespace', ['prod']]] as any,
        priorityclasses: [['globalDefault', ['Yes']], ['preemption', ['Never', 'PreemptLowerPriority']], ['value', ['7', '1000']], ['value', []]] as any,
        ingressclasses: [['controller', ['k8s.io/ingress-nginx']], ['default', ['Yes']], ['default', ['No', 'Yes']], ['controller', ['']]] as any,
        storageclasses: [['provisioner', ['p', 'ebs.csi.aws.com']], ['reclaimPolicy', ['Delete', 'Retain']], ['volumeBindingMode', ['WaitForFirstConsumer', '']], ['allowVolumeExpansion', ['Yes']]] as any,
        validatingwebhookconfigurations: [['failurePolicy', ['Fail']], ['failurePolicy', ['Ignore', 'Fail']], ['failurePolicy', ['Fail, Ignore']], ['failurePolicy', ['-']], ['webhooks', ['1']]] as any,
        mutatingwebhookconfigurations: [['failurePolicy', ['Fail']], ['failurePolicy', ['Ignore, Fail']], ['reinvocation', ['Never']], ['reinvocation', ['IfNeeded', 'Never']], ['reinvocation', ['-']]] as any,
        csidrivers: [['attachRequired', ['Yes']], ['attachRequired', ['No']], ['podInfoOnMount', ['No', 'Yes']], ['storageCapacity', ['Yes']], ['volumeModes', ['Persistent', '-']], ['volumeModes', ['Persistent, Ephemeral']]] as any,
        csinodes: [['driverNames', ['-']], ['driverNames', ['one, two']], ['driverCount', ['2']]] as any,
        persistentvolumes: [['capacity', ['1Gi', '10Gi']], ['accessModes', ['ReadWriteOnce']], ['reclaimPolicy', ['Delete', 'Retain']], ['status', ['Bound']], ['status', ['Available', 'Released']], ['claim', ['default/data-0']], ['claim', ['']], ['storageClass', ['standard', '']]] as any,
        clusterroles: [['aggregation', ['Yes']], ['aggregation', ['No']], ['rules', ['2']]] as any,
        clusterrolebindings: [['roleRef', ['ClusterRole/admin']], ['roleRef', ['ClusterRole/view', 'ClusterRole/admin']], ['subjects', ['1']]] as any,
        hpas: [['reference', ['Deployment/web']], ['reference', ['Deployment/web', 'StatefulSet/db']], ['minmax', ['0/4']], ['minmax', ['2/0', '1/3']], ['replicas', ['-']], ['replicas', ['3']]] as any,
        pdbs: [['minAvailable', ['-']], ['minAvailable', ['50%']], ['minAvailable', ['2']], ['maxUnavailable', ['-', '25%']], ['allowed', ['1']], ['allowed', ['-']]] as any,
        resourcequotas: [['resources', ['2']]] as any,
        limitranges: [['limits', ['2']]] as any,
        leases: [['holder', ['-']], ['holder', ['a', 'x']], ['duration', ['15']], ['holder', []]] as any,
        networkpolicies: [['podSelector', ['All Pods']], ['podSelector', ['app=web']], ['policyTypes', ['Ingress']], ['policyTypes', ['Ingress, Egress', 'Egress']]] as any,
        serviceaccounts: [['secrets', ['0']]] as any,
        persistentvolumeclaims: [['status', ['Bound']], ['status', ['Pending', 'Lost']], ['volume', ['pv-a']], ['capacity', ['1Gi', '10Gi']], ['accessModes', ['ReadWriteOnce']], ['accessModes', ['ReadOnlyMany, ReadWriteOnce']], ['storageClass', ['standard', '']], ['volumeMode', ['Block']], ['requested', ['5Gi']], ['selector', ['app=web']], ['namespace', ['prod']]] as any,
        deployments: [['strategy', ['Recreate']], ['strategy', ['RollingUpdate', 'Recreate']], ['image', ['nginx:1.25']], ['selector', ['app=web']], ['selector', ['']], ['replicas', ['3']], ['namespace', ['prod']]] as any,
        statefulsets: [['serviceName', ['db-headless']], ['serviceName', ['', 'cache']], ['updateStrategy', ['OnDelete']], ['updateStrategy', ['OnDelete', 'RollingUpdate']], ['image', ['nginx:1.25']], ['namespace', ['prod']]] as any,
        daemonsets: [['updateStrategy', ['RollingUpdate']], ['updateStrategy', ['OnDelete', 'RollingUpdate']], ['image', ['nginx:1.25']], ['selector', ['app=web']], ['nodeSelector', ['kubernetes.io/os=linux']], ['nodeSelector', ['']], ['desired', ['3']], ['namespace', ['prod']]] as any,
        jobs: [['controlledBy', ['CronJob']], ['controlledBy', ['CronJob', 'Rollout', '']], ['parallelism', ['1']], ['image', ['nginx:1.25']], ['startTime', ['']], ['namespace', ['prod']], ['completions', ['x']]] as any,
        replicasets: [['controlledBy', ['Deployment']], ['controlledBy', ['Deployment', 'Rollout', '']], ['controlledBy', ['ConfigMap']], ['desired', ['3']], ['namespace', ['prod']]] as any,
        roles: [['rules', ['2']]] as any,
        ingresses: [['status', ['Active']], ['status', ['Pending', 'No Rules']], ['class', ['nginx']], ['class', ['-', 'traefik']], ['hosts', ['*']], ['hosts', ['app.example.com']], ['address', ['34.1.2.3']], ['address', ['-']], ['paths', ['-']], ['paths', ['/', '/api']], ['tls', ['Yes']], ['tls', ['No']], ['defaultBackend', ['fallback']], ['defaultBackend', ['']], ['rules', ['1']]] as any,
        services: [['type', ['ClusterIP']], ['type', ['LoadBalancer', 'NodePort']], ['clusterIP', ['None']], ['clusterIP', ['10.96.0.1', '']], ['ports', ['80/TCP']], ['externalIP', ['34.1.2.3']], ['externalIP', ['']], ['selector', ['app=web']], ['sessionAffinity', ['None']], ['sessionAffinity', ['ClientIP']], ['externalName', ['db.example.com']]] as any,
        endpointslices: [['service', ['web']], ['service', ['-', 'db']], ['addressType', ['IPv4']], ['addressType', ['IPv6', '']], ['ports', ['80/TCP']], ['ports', ['-']]] as any,
        endpoints: [['ports', ['80/TCP']], ['ports', ['-', '80/TCP, 8080/TCP']], ['ports', ['53/UDP, 53/TCP']], ['endpoints', ['2']]] as any,
        rolebindings: [['roleRef', ['Role/admin']], ['roleRef', ['ClusterRole/view', 'Role/admin']], ['subjects', ['1']]] as any,
        pods: [['status', ['Running']], ['status', ['Running', 'Pending']], ['status', ['CrashLoopBackOff', 'Init:Error', 'Terminating']], ['status', ['Unknown']], ['controlledBy', ['Deployment']], ['controlledBy', ['Deployment', 'Rollout', '']], ['qos', ['Guaranteed', 'BestEffort']], ['qos', ['']], ['node', ['node-a']], ['node', ['']], ['podIP', ['10.1.0.5']], ['serviceAccount', ['default']], ['serviceAccount', ['builder', 'default']], ['priorityClass', ['low', '']], ['image', ['nginx:1.25']], ['namespace', ['prod']]] as any,
        nodes: [['conditions', ['Ready']], ['conditions', ['NotReady', 'SchedulingDisabled']], ['conditions', ['NotReady,SchedulingDisabled']], ['version', ['v1.29.3']], ['version', ['']], ['roles', ['control-plane']], ['roles', ['control-plane, etcd, zeta', '']], ['internalIP', ['10.0.0.5']], ['externalIP', ['']], ['osImage', ['Ubuntu 22.04']], ['kernelVersion', ['5.15.0-1', '6.1.0']], ['containerRuntime', ['containerd://1.7.2']], ['architecture', ['arm64']], ['os', ['linux']]] as any,
        events: [['type', ['Warning']], ['type', ['Normal', 'Warning']], ['namespace', ['default']], ['reason', ['BackOff']], ['source', ['kubelet', '']]] as any,
    };
    for (const [col, values] of (selects[type] || []) as any) {
        cases.push({ name: `${col} select ${values.join('|') || 'empty'}`, filters: { [col]: { type: 'select', values } } });
    }
    if (type === 'configmaps') {
        cases.push({ name: 'keyCount numeric', filters: { keyCount: { type: 'numeric', conditions: [{ operator: '>=', value: 2 }] } } });
        cases.push({ name: 'size numeric', filters: { size: { type: 'numeric', conditions: [{ operator: '>', value: 30 }] } } });
        cases.push({ name: 'binaryKeys regex', filters: { binaryKeys: { type: 'regex', conditions: ['^[1-9]'] } } });
        cases.push({ name: 'keys regex', filters: { keys: { type: 'regex', conditions: ['config', '10'], logic: 'or' } } });
    }
    if (type === 'events') {
        cases.push({ name: 'count numeric', filters: { count: { type: 'numeric', conditions: [{ operator: '>', value: 5 }] } } });
        cases.push({ name: 'message regex', filters: { message: { type: 'regex', conditions: ['failed'] } } });
        cases.push({ name: 'involvedObject regex', filters: { involvedObject: { type: 'regex', conditions: ['^pod/', '^Node/'], logic: 'or' } } });
    }
    if (type === 'priorityclasses') {
        cases.push({ name: 'value regex on number', filters: { value: { type: 'regex', conditions: ['^1'] } } });
    }
    if (type === 'validatingwebhookconfigurations' || type === 'mutatingwebhookconfigurations') {
        cases.push({ name: 'webhooks numeric', filters: { webhooks: { type: 'numeric', conditions: [{ operator: '>=', value: 2 }] } } });
        cases.push({ name: 'failurePolicy regex', filters: { failurePolicy: { type: 'regex', conditions: ['^Ignore'] } } });
    }
    if (type === 'csinodes') {
        cases.push({ name: 'driverCount numeric', filters: { driverCount: { type: 'numeric', conditions: [{ operator: '>', value: 1 }] } } });
    }
    if (type === 'clusterroles') {
        cases.push({ name: 'rules numeric', filters: { rules: { type: 'numeric', conditions: [{ operator: '<=', value: 1 }] } } });
    }
    if (type === 'clusterrolebindings') {
        cases.push({ name: 'subjects numeric', filters: { subjects: { type: 'numeric', conditions: [{ operator: '=', value: 0 }] } } });
        cases.push({ name: 'roleRef regex', filters: { roleRef: { type: 'regex', conditions: ['^ClusterRole/a'] } } });
    }
    if (type === 'persistentvolumes') {
        cases.push({ name: 'claim regex', filters: { claim: { type: 'regex', conditions: ['^prod/'] } } });
    }
    if (type === 'secrets') {
        cases.push({ name: 'type regex', filters: { type: { type: 'regex', conditions: ['^kubernetes'] } } });
    }
    if (NAMESPACE_REGEX_TYPES.has(type)) {
        cases.push({ name: 'namespace regex', filters: { namespace: { type: 'regex', conditions: ['^team'] } } });
        cases.push({ name: 'namespace regex and name', filters: { namespace: { type: 'regex', conditions: ['prod', 'e'], logic: 'or' }, name: { type: 'regex', conditions: ['a'] } } });
    }
    if (type === 'hpas') {
        cases.push({ name: 'reference regex', filters: { reference: { type: 'regex', conditions: ['^Deployment/'] } } });
        cases.push({ name: 'replicas numeric', filters: { replicas: { type: 'numeric', conditions: [{ operator: '>=', value: 3 }] } } });
    }
    if (type === 'pdbs') {
        cases.push({ name: 'minAvailable regex', filters: { minAvailable: { type: 'regex', conditions: ['%$'] } } });
        cases.push({ name: 'allowed numeric', filters: { allowed: { type: 'numeric', conditions: [{ operator: '>', value: 0 }] } } });
        cases.push({ name: 'minAvailable numeric', filters: { minAvailable: { type: 'numeric', conditions: [{ operator: '>=', value: 2 }] } } });
    }
    if (type === 'resourcequotas') {
        cases.push({ name: 'resources numeric', filters: { resources: { type: 'numeric', conditions: [{ operator: '>=', value: 2 }] } } });
    }
    if (type === 'limitranges') {
        cases.push({ name: 'limits numeric', filters: { limits: { type: 'numeric', conditions: [{ operator: '=', value: 0 }] } } });
    }
    if (type === 'leases') {
        cases.push({ name: 'holder regex', filters: { holder: { type: 'regex', conditions: ['^node'] } } });
        cases.push({ name: 'renewTime regex', filters: { renewTime: { type: 'regex', conditions: ['T11:5'] } } });
        cases.push({ name: 'duration numeric', filters: { duration: { type: 'numeric', conditions: [{ operator: '>', value: 15 }] } } });
    }
    if (type === 'networkpolicies') {
        cases.push({ name: 'podSelector regex', filters: { podSelector: { type: 'regex', conditions: ['\\+\\d$'] } } });
    }
    if (type === 'endpoints') {
        cases.push({ name: 'endpoints numeric', filters: { endpoints: { type: 'numeric', conditions: [{ operator: '>=', value: 3 }] } } });
        cases.push({ name: 'ports regex', filters: { ports: { type: 'regex', conditions: ['\\+\\d$'] } } });
    }
    if (type === 'endpointslices') {
        cases.push({ name: 'endpoints numeric', filters: { endpoints: { type: 'numeric', conditions: [{ operator: '>=', value: 2 }] } } });
        cases.push({ name: 'service regex', filters: { service: { type: 'regex', conditions: ['^w'] } } });
        cases.push({ name: 'ports regex', filters: { ports: { type: 'regex', conditions: ['undefined'] } } });
    }
    if (type === 'services') {
        cases.push({ name: 'type regex', filters: { type: { type: 'regex', conditions: ['^Load|Port$'] } } });
        cases.push({ name: 'clusterIP regex', filters: { clusterIP: { type: 'regex', conditions: ['^10\\.96\\.0'] } } });
        cases.push({ name: 'ports regex', filters: { ports: { type: 'regex', conditions: ['undefined'] } } });
        cases.push({ name: 'selector regex', filters: { selector: { type: 'regex', conditions: ['app=web'] } } });
    }
    if (type === 'ingresses') {
        cases.push({ name: 'hosts regex', filters: { hosts: { type: 'regex', conditions: ['example\\.com, '] } } });
        cases.push({ name: 'address regex', filters: { address: { type: 'regex', conditions: ['^34\\.'] } } });
        cases.push({ name: 'paths regex', filters: { paths: { type: 'regex', conditions: ['\\.\\.\\.$'] } } });
        cases.push({ name: 'rules numeric', filters: { rules: { type: 'numeric', conditions: [{ operator: '>=', value: 2 }] } } });
        cases.push({ name: 'status and class', filters: { status: { type: 'select', values: ['Active'] }, class: { type: 'select', values: ['nginx'] } } });
    }
    if (type === 'roles') {
        cases.push({ name: 'rules numeric', filters: { rules: { type: 'numeric', conditions: [{ operator: '<=', value: 1 }] } } });
    }
    if (type === 'rolebindings') {
        cases.push({ name: 'subjects numeric', filters: { subjects: { type: 'numeric', conditions: [{ operator: '=', value: 0 }] } } });
        cases.push({ name: 'roleRef regex', filters: { roleRef: { type: 'regex', conditions: ['^ClusterRole/a'] } } });
    }
    if (type === 'replicasets') {
        cases.push({ name: 'desired numeric', filters: { desired: { type: 'numeric', conditions: [{ operator: '>=', value: 3 }] } } });
        cases.push({ name: 'ready numeric', filters: { ready: { type: 'numeric', conditions: [{ operator: '=', value: 0 }] } } });
        cases.push({ name: 'controlledBy regex', filters: { controlledBy: { type: 'regex', conditions: ['^Dep', 'Roll'], logic: 'or' } } });
    }
    if (type === 'jobs') {
        cases.push({ name: 'duration numeric', filters: { duration: { type: 'numeric', conditions: [{ operator: '>', value: 0 }] } } });
        cases.push({ name: 'parallelism numeric', filters: { parallelism: { type: 'numeric', conditions: [{ operator: '>=', value: 2 }] } } });
        cases.push({ name: 'startTime regex', filters: { startTime: { type: 'regex', conditions: ['T1[0-9]:'] } } });
        cases.push({ name: 'image regex', filters: { image: { type: 'regex', conditions: ['^nginx'] } } });
        cases.push({ name: 'controlledBy regex', filters: { controlledBy: { type: 'regex', conditions: ['^Cron'] } } });
    }
    if (type === 'daemonsets') {
        cases.push({ name: 'desired numeric', filters: { desired: { type: 'numeric', conditions: [{ operator: '>=', value: 3 }] } } });
        cases.push({ name: 'upToDate numeric', filters: { upToDate: { type: 'numeric', conditions: [{ operator: '=', value: 0 }] } } });
        cases.push({ name: 'selector regex', filters: { selector: { type: 'regex', conditions: ['app=web,tier'] } } });
        cases.push({ name: 'nodeSelector regex', filters: { nodeSelector: { type: 'regex', conditions: ['disk=ssd'] } } });
        cases.push({ name: 'image regex', filters: { image: { type: 'regex', conditions: ['^busybox'] } } });
    }
    if (type === 'deployments') {
        cases.push({ name: 'ready numeric', filters: { ready: { type: 'numeric', conditions: [{ operator: '>=', value: 3 }] } } });
        cases.push({ name: 'replicas numeric', filters: { replicas: { type: 'numeric', conditions: [{ operator: '=', value: 1 }] } } });
        cases.push({ name: 'availableReplicas numeric', filters: { availableReplicas: { type: 'numeric', conditions: [{ operator: '>', value: 0 }] } } });
        cases.push({ name: 'selector regex', filters: { selector: { type: 'regex', conditions: ['app=web,tier'] } } });
        cases.push({ name: 'image regex', filters: { image: { type: 'regex', conditions: ['^busybox'] } } });
        cases.push({ name: 'strategy regex', filters: { strategy: { type: 'regex', conditions: ['^Rec'] } } });
    }
    if (type === 'statefulsets') {
        cases.push({ name: 'ready numeric', filters: { ready: { type: 'numeric', conditions: [{ operator: '>=', value: 2 }] } } });
        cases.push({ name: 'volumeClaims numeric', filters: { volumeClaims: { type: 'numeric', conditions: [{ operator: '>', value: 0 }] } } });
        cases.push({ name: 'serviceName regex', filters: { serviceName: { type: 'regex', conditions: ['^db', 'Svc'], logic: 'or' } } });
        cases.push({ name: 'image regex', filters: { image: { type: 'regex', conditions: ['^busybox'] } } });
    }
    if (type === 'persistentvolumeclaims') {
        cases.push({ name: 'capacity regex', filters: { capacity: { type: 'regex', conditions: ['Gi$'] } } });
        cases.push({ name: 'volume regex', filters: { volume: { type: 'regex', conditions: ['^pv-'] } } });
        cases.push({ name: 'storageClass regex', filters: { storageClass: { type: 'regex', conditions: ['^[a-z]'] } } });
    }
    if (type === 'pods') {
        cases.push({ name: 'restarts numeric', filters: { restarts: { type: 'numeric', conditions: [{ operator: '>=', value: 3 }] } } });
        cases.push({ name: 'restarts zero', filters: { restarts: { type: 'numeric', conditions: [{ operator: '=', value: 0 }] } } });
        cases.push({ name: 'node regex', filters: { node: { type: 'regex', conditions: ['^node-'] } } });
        cases.push({ name: 'podIP regex', filters: { podIP: { type: 'regex', conditions: ['^10\\.1\\.[01]\\.'] } } });
        cases.push({ name: 'status regex', filters: { status: { type: 'regex', conditions: ['^Init', 'Back'], logic: 'or' } } });
        cases.push({ name: 'controlledBy regex', filters: { controlledBy: { type: 'regex', conditions: ['^Dep', 'Roll'], logic: 'or' } } });
    }
    if (type === 'nodes') {
        cases.push({ name: 'version regex', filters: { version: { type: 'regex', conditions: ['^v1\\.2'] } } });
        cases.push({ name: 'roles regex', filters: { roles: { type: 'regex', conditions: ['control', 'etcd'], logic: 'or' } } });
        cases.push({ name: 'conditions regex', filters: { conditions: { type: 'regex', conditions: ['Pressure'] } } });
        cases.push({ name: 'internalIP regex', filters: { internalIP: { type: 'regex', conditions: ['^10\\.0\\.0\\.'] } } });
    }
    if (type === 'serviceaccounts') {
        cases.push({ name: 'secrets numeric', filters: { secrets: { type: 'numeric', conditions: [{ operator: '>=', value: 2 }] } } });
    }
    cases.push({
        name: 'combined name and age',
        filters: { name: { type: 'regex', conditions: ['a'] }, ...(has('age') ? { age: { type: 'numeric', conditions: [{ operator: '>', value: 5 }] } } : {}) },
    });
    return cases;
}

/** One state of a metrics overlay: the legacy transform has run over the raw result and the list semantics read its output. */
interface OverlayStateSetup {
    name: string;
    available: boolean | null;
    raw: any;
    semantics: Record<string, any>;
    /** Records the Go overlay source must publish for the state, keyed as the spec keys them. */
    records: Record<string, string>;
}

interface OverlaySetup {
    name: string;
    /** Columns whose cells the overlay feeds (volatile and display-only). */
    columns: string[];
    /** Columns whose sort order and numeric filters are recorded per state. */
    sortColumns: string[];
    states: OverlayStateSetup[];
}

/** One state of the pod list behind a derived column: the legacy semantics are built over exactly these pods. */
interface DerivedStateSetup {
    name: string;
    pods: any[];
    semantics: Record<string, any>;
}

interface DerivedSetup {
    /** The source type the columns are derived from. */
    from: string;
    /** Columns whose cells depend on the pod list. */
    columns: string[];
    /** Columns whose sort order is recorded per state. */
    sortColumns: string[];
    states: DerivedStateSetup[];
}

interface TypeSetup {
    resourceType: string;
    fixtures: FixtureSet;
    semantics: Record<string, any>;
    overlay?: OverlaySetup;
    derived?: DerivedSetup;
}

function overlayStates<R extends { name: string; available: boolean | null; stale?: boolean }>(
    states: R[],
    transform: (st: R) => Record<string, any>,
    makeSemantics: (lookup: (item: any) => any) => Record<string, any>,
    keyOf: (item: any) => string,
    fields: readonly string[],
    withRaw: (st: R) => any,
): OverlayStateSetup[] {
    return states.map(st => {
        const map = transform(st);
        const records: Record<string, string> = {};
        for (const [k, m] of Object.entries(map)) records[k] = metricsRecordText(recordOf(fields as any, m as any));
        return {
            name: st.name,
            available: st.available,
            raw: withRaw(st),
            semantics: makeSemantics(item => map[keyOf(item)]),
            records,
        };
    });
}

const podSet = podFixtures();
const nodeSet = nodeFixtures();
const namespaceSet = namespaceFixtures();
const nodeKey = (item: any) => item.metadata?.name;

const podOverlay: OverlaySetup = {
    name: 'metrics',
    columns: ['cpu', 'memory', 'metrics'],
    sortColumns: ['cpu', 'memory', 'metrics'],
    states: overlayStates(podOverlayStates(podSet.items), (st: OverlayRaw) => (st.raw && (st.raw.available || st.stale) ? transformPodMetrics(st.raw) : {}),
        createPodSemantics, podMetricKey, POD_RECORD_FIELDS, st => st.raw),
};

const nodeOverlay: OverlaySetup = {
    name: 'metrics',
    columns: ['cpu', 'memory', 'pods', 'metrics'],
    sortColumns: ['cpu', 'memory', 'pods', 'metrics'],
    states: overlayStates(nodeOverlayStates(nodeSet.items), (st: OverlayRaw) => (st.raw && (st.raw.available || st.stale) ? transformNodeMetrics(st.raw) : {}),
        createNodeSemantics, nodeKey, NODE_RECORD_FIELDS, st => st.raw),
};

const namespaceOverlay: OverlaySetup = {
    name: 'metrics',
    columns: ['cpu', 'memory', 'metrics'],
    sortColumns: ['cpu', 'memory', 'metrics'],
    states: overlayStates(namespaceOverlayStates(namespaceSet.items),
        (st: NamespaceOverlayRaw) => (st.raw ? aggregateNamespaceMetrics(transformPodMetrics(st.raw.pods), transformNodeMetrics(st.raw.nodes)) : {}),
        createNamespaceSemantics, nodeKey, NAMESPACE_RECORD_FIELDS, st => st.raw),
};

const deploymentSet = deploymentFixtures();
const statefulSetSet = statefulSetFixtures();

const deploymentDerived: DerivedSetup = {
    from: 'pods',
    columns: ['pods', 'podRefs'],
    sortColumns: ['pods'],
    states: workloadPodStates(deploymentSet.items).map(st => ({
        name: st.name, pods: st.pods, semantics: createDeploymentSemantics(deploymentPodMaps(deploymentSet.items, st.pods)),
    })),
};

const statefulSetDerived: DerivedSetup = {
    from: 'pods',
    columns: ['pods', 'podRefs'],
    sortColumns: ['pods'],
    states: workloadPodStates(statefulSetSet.items).map(st => ({
        name: st.name, pods: st.pods, semantics: createStatefulSetSemantics(item => getDeploymentPods(item, st.pods)),
    })),
};

const SETUPS: TypeSetup[] = [
    { resourceType: 'namespaces', fixtures: namespaceSet, semantics: namespaceSemantics, overlay: namespaceOverlay },
    { resourceType: 'configmaps', fixtures: configMapFixtures(), semantics: configMapSemantics },
    { resourceType: 'secrets', fixtures: secretFixtures(), semantics: secretSemantics },
    { resourceType: 'events', fixtures: eventFixtures(), semantics: eventSemantics },
    { resourceType: 'priorityclasses', fixtures: priorityClassFixtures(), semantics: priorityClassSemantics },
    { resourceType: 'ingressclasses', fixtures: ingressClassFixtures(), semantics: ingressClassSemantics },
    { resourceType: 'storageclasses', fixtures: storageClassFixtures(), semantics: storageClassSemantics },
    { resourceType: 'validatingwebhookconfigurations', fixtures: validatingWebhookFixtures(), semantics: validatingWebhookSemantics },
    { resourceType: 'mutatingwebhookconfigurations', fixtures: mutatingWebhookFixtures(), semantics: mutatingWebhookSemantics },
    { resourceType: 'csidrivers', fixtures: csiDriverFixtures(), semantics: csiDriverSemantics },
    { resourceType: 'csinodes', fixtures: csiNodeFixtures(), semantics: csiNodeSemantics },
    { resourceType: 'persistentvolumes', fixtures: pvFixtures(), semantics: pvSemantics },
    { resourceType: 'clusterroles', fixtures: clusterRoleFixtures(), semantics: clusterRoleSemantics },
    { resourceType: 'clusterrolebindings', fixtures: clusterRoleBindingFixtures(), semantics: clusterRoleBindingSemantics },
    { resourceType: 'hpas', fixtures: hpaFixtures(), semantics: hpaSemantics },
    { resourceType: 'pdbs', fixtures: pdbFixtures(), semantics: pdbSemantics },
    { resourceType: 'resourcequotas', fixtures: resourceQuotaFixtures(), semantics: resourceQuotaSemantics },
    { resourceType: 'limitranges', fixtures: limitRangeFixtures(), semantics: limitRangeSemantics },
    { resourceType: 'leases', fixtures: leaseFixtures(), semantics: leaseSemantics },
    { resourceType: 'networkpolicies', fixtures: networkPolicyFixtures(), semantics: networkPolicySemantics },
    { resourceType: 'serviceaccounts', fixtures: serviceAccountFixtures(), semantics: serviceAccountSemantics },
    { resourceType: 'endpoints', fixtures: endpointsFixtures(), semantics: endpointsSemantics },
    { resourceType: 'endpointslices', fixtures: endpointSliceFixtures(), semantics: endpointSliceSemantics },
    { resourceType: 'services', fixtures: serviceFixtures(), semantics: serviceSemantics },
    { resourceType: 'ingresses', fixtures: ingressFixtures(), semantics: ingressSemantics },
    { resourceType: 'roles', fixtures: roleFixtures(), semantics: roleSemantics },
    { resourceType: 'rolebindings', fixtures: roleBindingFixtures(), semantics: roleBindingSemantics },
    { resourceType: 'replicasets', fixtures: replicaSetFixtures(), semantics: replicaSetSemantics },
    { resourceType: 'jobs', fixtures: jobFixtures(), semantics: jobSemantics },
    { resourceType: 'daemonsets', fixtures: daemonSetFixtures(), semantics: daemonSetSemantics },
    { resourceType: 'persistentvolumeclaims', fixtures: pvcFixtures(), semantics: pvcSemantics },
    { resourceType: 'deployments', fixtures: deploymentSet, semantics: deploymentSemantics, derived: deploymentDerived },
    { resourceType: 'statefulsets', fixtures: statefulSetSet, semantics: statefulSetSemantics, derived: statefulSetDerived },
    { resourceType: 'pods', fixtures: podSet, semantics: podSemantics, overlay: podOverlay },
    { resourceType: 'nodes', fixtures: nodeSet, semantics: nodeSemantics, overlay: nodeOverlay },
];

const nul = (v: any) => (v === undefined || (typeof v === 'number' && Number.isNaN(v)) ? null : v);

function revive(filters: Record<string, any>) {
    const out: Record<string, any> = {};
    for (const [k, f] of Object.entries(filters)) {
        out[k] = { ...f, ...(f.values ? { values: new Set<string>(f.values) } : {}) };
    }
    return out;
}

export function buildGolden(setup: TypeSetup) {
    const { resourceType, fixtures, semantics } = setup;
    const items = fixtures.items;
    const columns = semanticColumns(semantics);
    const keys = columns.map(c => c.key);
    const uid = (i: any) => i.metadata.uid;
    const sortable = items.filter(i => !fixtures.noSort.includes(uid(i)));

    const cells: Record<string, Record<string, any>> = {};
    const filterValues: Record<string, Record<string, any>> = {};
    const numeric: Record<string, Record<string, any>> = {};
    for (const item of items) {
        cells[uid(item)] = {};
        filterValues[uid(item)] = {};
        numeric[uid(item)] = {};
        for (const col of columns) {
            cells[uid(item)][col.key] = nul(col.getValue?.(item));
            filterValues[uid(item)][col.key] = getColumnValue(col, item);
            numeric[uid(item)][col.key] = nul(getNumericValue(col, item));
        }
    }

    const sortOrders: Record<string, Record<string, string[]>> = {};
    for (const key of keys) {
        sortOrders[key] = {};
        for (const direction of ['asc', 'desc']) {
            sortOrders[key][direction] = sortItems(sortable, columns, { key, direction }).map(uid);
        }
    }

    const searchCases = [...COMMON_QUERIES, ...(TYPE_QUERIES[resourceType] ?? [])].map(query => {
        const filter = createFilter(resourceType, query);
        return { query, matches: items.filter(filter).map(uid) };
    });

    const columnFilterCases = filterCases(resourceType, keys).map(c => {
        const predicate = compileColumnFilters(columns, revive(c.filters));
        const matches = (predicate ? items.filter(predicate) : items).map(uid);
        return { name: c.name, filters: c.filters, matches };
    });

    const filterTypes = detectColumnFilterTypes(columns);
    const uniqueValues: Record<string, any> = {};
    for (const col of columns) {
        if (filterTypes[col.key] !== 'select') continue;
        uniqueValues[col.key] = computeColumnUniqueValues(col, items, resourceType) ?? null;
    }

    const overlay = setup.overlay ? buildOverlayGolden(setup, sortable) : undefined;
    const derived = setup.derived ? buildDerivedGolden(setup, sortable) : undefined;

    return {
        type: resourceType,
        now: GOLDEN_NOW,
        columns: keys,
        filterTypes,
        fixtures: items,
        noSort: fixtures.noSort,
        cells,
        filterValues,
        numeric,
        sortOrders,
        searchCases,
        columnFilterCases,
        uniqueValues,
        overlay,
        derived,
    };
}

/**
 * The derived half of a golden file: for every pod list the cells, numeric values and sort orders
 * of the columns that read the pods, computed by the legacy code with the pod list behind the
 * list semantics. The raw pods are recorded so the Go index is fed exactly the same input.
 */
function buildDerivedGolden(setup: TypeSetup, sortable: any[]) {
    const derived = setup.derived!;
    const items = setup.fixtures.items;
    const uid = (i: any) => i.metadata.uid;
    const states = derived.states.map(st => {
        const columns = semanticColumns(st.semantics);
        const cells: Record<string, Record<string, any>> = {};
        const numeric: Record<string, Record<string, any>> = {};
        for (const item of items) {
            cells[uid(item)] = {};
            numeric[uid(item)] = {};
            for (const col of columns.filter(c => derived.columns.includes(c.key))) {
                cells[uid(item)][col.key] = nul(col.getValue?.(item));
                numeric[uid(item)][col.key] = nul(getNumericValue(col, item));
            }
        }
        const sortOrders: Record<string, Record<string, string[]>> = {};
        for (const key of derived.sortColumns) {
            sortOrders[key] = {};
            for (const direction of ['asc', 'desc']) sortOrders[key][direction] = sortItems(sortable, columns, { key, direction }).map(uid);
        }
        return { name: st.name, pods: st.pods, cells, numeric, sortOrders };
    });
    return { from: derived.from, columns: derived.columns, states };
}

/** Numeric and regex filters on the overlay's columns, plus one that combines them with a name filter. */
function overlayFilterCases(type: string, columns: string[]): FilterCase[] {
    const num = (col: string, conditions: any[], logic?: string): FilterCase['filters'] => ({ [col]: { type: 'numeric', conditions, ...(logic ? { logic } : {}) } });
    const cases: FilterCase[] = [];
    for (const col of columns.filter(c => c === 'cpu' || c === 'memory')) {
        cases.push({ name: `${col} at least 10`, filters: num(col, [{ operator: '>=', value: 10 }]) });
        cases.push({ name: `${col} below 5`, filters: num(col, [{ operator: '<', value: 5 }]) });
        cases.push({ name: `${col} between`, filters: num(col, [{ operator: '>', value: 2 }, { operator: '<=', value: 40 }], 'and') });
        cases.push({ name: `${col} either end`, filters: num(col, [{ operator: '<', value: 1 }, { operator: '>=', value: 50 }], 'or') });
        cases.push({ name: `${col} equals zero`, filters: num(col, [{ operator: '=', value: 0 }]) });
        cases.push({ name: `${col} not equal five`, filters: num(col, [{ operator: '!=', value: 5 }]) });
        cases.push({ name: `${col} equals negative one`, filters: num(col, [{ operator: '=', value: -1 }]) });
        cases.push({ name: `${col} select empty`, filters: { [col]: { type: 'select', values: ['x'] } } });
        cases.push({ name: `${col} regex on a number`, filters: { [col]: { type: 'regex', conditions: ['^1'] } } });
    }
    cases.push({ name: 'metrics numeric and name', filters: { ...num(columns[0], [{ operator: '>=', value: 3 }]), name: { type: 'regex', conditions: ['a'] } } });
    if (columns.includes('cpu') && columns.includes('memory')) {
        cases.push({ name: 'cpu and memory', filters: { ...num('cpu', [{ operator: '>=', value: 1 }]), ...num('memory', [{ operator: '>=', value: 1 }]) } });
    }
    void type;
    return cases;
}

/**
 * The overlay half of the golden file: for every metrics state the cells, filter values, numeric
 * values, sort orders and column-filter matches of the overlay's columns, computed by the legacy
 * code with the state's metrics map behind the list semantics.
 */
function buildOverlayGolden(setup: TypeSetup, sortable: any[]) {
    const overlay = setup.overlay!;
    const items = setup.fixtures.items;
    const uid = (i: any) => i.metadata.uid;
    const states = overlay.states.map(st => {
        const columns = semanticColumns(st.semantics);
        const cells: Record<string, Record<string, any>> = {};
        const filterValues: Record<string, Record<string, any>> = {};
        const numeric: Record<string, Record<string, any>> = {};
        for (const item of items) {
            cells[uid(item)] = {};
            filterValues[uid(item)] = {};
            numeric[uid(item)] = {};
            for (const col of columns.filter(c => overlay.columns.includes(c.key))) {
                cells[uid(item)][col.key] = nul(col.getValue?.(item));
                filterValues[uid(item)][col.key] = getColumnValue(col, item);
                numeric[uid(item)][col.key] = nul(getNumericValue(col, item));
            }
        }
        const sortOrders: Record<string, Record<string, string[]>> = {};
        for (const key of overlay.sortColumns) {
            sortOrders[key] = {};
            for (const direction of ['asc', 'desc']) sortOrders[key][direction] = sortItems(sortable, columns, { key, direction }).map(uid);
        }
        const columnFilterCases = overlayFilterCases(setup.resourceType, overlay.columns).map(c => {
            const predicate = compileColumnFilters(columns, revive(c.filters));
            return { name: c.name, filters: c.filters, matches: (predicate ? items.filter(predicate) : items).map(uid) };
        });
        return { name: st.name, available: st.available, raw: st.raw, records: st.records, cells, filterValues, numeric, sortOrders, columnFilterCases };
    });
    return { name: overlay.name, columns: overlay.columns, states };
}

/** Compact, stable JSON: objects and arrays of objects expand, leaf arrays stay on one line. */
export function stringify(value: any, indent = ''): string {
    if (value === null || typeof value !== 'object') return JSON.stringify(value);
    const next = `${indent} `;
    if (Array.isArray(value)) {
        if (value.every(v => v === null || typeof v !== 'object')) return JSON.stringify(value);
        return `[\n${value.map(v => next + stringify(v, next)).join(',\n')}\n${indent}]`;
    }
    const entries = Object.entries(value).filter(([, v]) => v !== undefined);
    if (entries.length === 0) return '{}';
    return `{\n${entries.map(([k, v]) => `${next}${JSON.stringify(k)}: ${stringify(v, next)}`).join(',\n')}\n${indent}}`;
}

const localeIsEnglish = new Intl.Collator().resolvedOptions().locale.toLowerCase().startsWith('en');

describe('golden parity files', () => {
    beforeAll(() => {
        vi.useFakeTimers({ now: GOLDEN_NOW, toFake: ['Date'] });
    });
    afterAll(() => {
        vi.useRealTimers();
    });

    it('sorts with an English-compatible default collator', () => {
        // The Go side is built against the root/English CLDR collation. Another default locale
        // would make recorded orders machine dependent.
        expect(typeof compareNames('a', 'b')).toBe('number');
    });

    for (const setup of SETUPS) {
        it(`${setup.resourceType} matches the committed golden file`, () => {
            const text = `${stringify(buildGolden(setup))}\n`;
            if (regenerate()) {
                writeGolden(setup.resourceType, text);
                return;
            }
            if (!localeIsEnglish) {
                console.warn(`skipping golden drift check: default locale is not English`);
                return;
            }
            const committed = readGolden(setup.resourceType);
            expect(committed, `missing golden file for ${setup.resourceType}; run GOLDEN=1 npx vitest run src/test/golden`).not.toBeNull();
            expect(text === committed, `${setup.resourceType}.json is stale; run GOLDEN=1 npx vitest run src/test/golden`).toBe(true);
        });
    }
});
