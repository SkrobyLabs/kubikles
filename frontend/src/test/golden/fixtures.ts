// Fixtures for the golden-parity files. Shapes follow what the Go backend serialises for the
// legacy list bindings (typed objects through encoding/json: zero timestamps are null, empty
// omitempty fields are absent), plus hand-written edge cases. Everything is deterministic.

export const GOLDEN_NOW = Date.UTC(2025, 5, 1, 12, 0, 0);

function prng(seed: number) {
    let a = seed >>> 0;
    return () => {
        a = (a + 0x6d2b79f5) >>> 0;
        let t = a;
        t = Math.imul(t ^ (t >>> 15), t | 1);
        t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
        return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
}

const iso = (offsetMinutes: number) => new Date(GOLDEN_NOW - offsetMinutes * 60000).toISOString().replace('.000Z', 'Z');
const pick = <T,>(r: () => number, arr: T[]): T => arr[Math.floor(r() * arr.length)];

// Names chosen to exercise collation, case, digits, non-ASCII and supplementary-plane text.
const NAMES = [
    'alpha', 'Alpha', 'ALPHA', 'beta', 'Beta-2', 'beta-10', 'beta-9', '10-item', '9-item', 'item-a', 'item_b',
    'kube-system', 'kube-public', 'default', 'x', 'X', 'zeta', 'eta.v1', 'eta-v1', 'eta v2',
    'cafe', 'café', 'Café', 'zebra', 'éclair', 'Éclair', 'resume', 'résumé',
    '日本語', 'emoji-😀', 'Ａfullwidth', 'name.with.dots',
];
const NAMESPACES = ['default', 'kube-system', 'prod', 'Prod', 'team-a', 'team-b', 'étude'];

// Same timestamp, same namespace: ordering falls through to the name collator, so these names
// differ only in case, accents and punctuation.
const TIE_NAMES = ['tie-a', 'tie-A', 'tie-\u00e1', 'tie-\u00e0', 'tie-\u00e4', 'tie-b', 'Tie-b', 'tie_b', 'tie b', 'tie-10', 'tie-2', 'tie\u00a0z', 'ti\u00e9'];
function tieGroup(ns: string | undefined, build: (name: string, meta: any) => any): any[] {
    return TIE_NAMES.map((name, k) => {
        const meta: any = { name, uid: `uid-tie-${String(k).padStart(2, '0')}`, creationTimestamp: iso(3000) };
        if (ns !== undefined) meta.namespace = ns;
        return build(name, meta);
    });
}

export interface FixtureSet {
    type: string;
    items: any[];
    /** uids whose rows are excluded from sort goldens (mixed undefined sort keys are algorithm dependent). */
    noSort: string[];
}

function metadata(r: () => number, i: number, name: string, ns: string | undefined, extra: any = {}) {
    const m: any = { name, uid: `uid-${String(i).padStart(4, '0')}` };
    if (ns !== undefined) m.namespace = ns;
    m.creationTimestamp = iso(Math.floor(r() * 4000) + 1);
    return { ...m, ...extra };
}

/** Sort order the Go side uses as the final tie-break, so legacy stable sorts line up. */
function canonical(items: any[]): any[] {
    return [...items].sort((a, b) => {
        const an = a.metadata.namespace ?? '', bn = b.metadata.namespace ?? '';
        if (an !== bn) return an < bn ? -1 : 1;
        const au = a.metadata.uid, bu = b.metadata.uid;
        return au < bu ? -1 : au > bu ? 1 : 0;
    });
}

export function namespaceFixtures(): FixtureSet {
    const r = prng(11);
    const items: any[] = [];
    const phases = ['Active', 'Active', 'Active', 'Terminating', 'Weird'];
    NAMES.forEach((name, i) => {
        const labels = r() > 0.5 ? { app: pick(r, ['web', 'db', 'cache']), '10': 'ten', '9': 'nine', tier: 'x' } : undefined;
        const item: any = {
            metadata: metadata(r, i, name, undefined, labels ? { labels } : {}),
            spec: r() > 0.8 ? { finalizers: ['kubernetes'] } : {},
            status: { phase: pick(r, phases) },
        };
        items.push(item);
    });
    // Edge cases
    items.push({ metadata: { name: 'no-status', uid: 'uid-edge-1', creationTimestamp: iso(5) }, spec: {}, status: {} });
    items.push({ metadata: { name: 'terminating-ts', uid: 'uid-edge-2', creationTimestamp: iso(7), deletionTimestamp: iso(1) }, spec: {}, status: { phase: 'Terminating' } });
    const nots: any = { metadata: { name: 'no-timestamp', uid: 'uid-edge-3', creationTimestamp: null }, spec: {}, status: { phase: 'Active' } };
    items.push(nots);
    items.push({ metadata: { name: 'annotated', uid: 'uid-edge-4', creationTimestamp: iso(9), annotations: { note: 'Hello World', 'a/b': 'c=d' } }, spec: {}, status: { phase: 'Active' } });
    // Same timestamp ties
    const tie = iso(100);
    items.push({ metadata: { name: 'tie-b', uid: 'uid-edge-5', creationTimestamp: tie }, spec: {}, status: { phase: 'Active' } });
    items.push({ metadata: { name: 'tie-a', uid: 'uid-edge-6', creationTimestamp: tie }, spec: {}, status: { phase: 'Active' } });
    items.push(...tieGroup(undefined, (_n, metadata) => ({ metadata, spec: {}, status: { phase: 'Active' } })));
    return { type: 'namespaces', items: canonical(items), noSort: ['uid-edge-3'] };
}

export function configMapFixtures(): FixtureSet {
    const r = prng(23);
    const items: any[] = [];
    const keysPool = ['a.yaml', 'config', 'z', '10', '9', '100', 'key with space', 'kéy', 'KEY', 'dup'];
    NAMES.forEach((name, i) => {
        const ns = pick(r, NAMESPACES);
        const data: any = {};
        const n = Math.floor(r() * 5);
        for (let k = 0; k < n; k++) data[pick(r, keysPool)] = pick(r, ['v', 'value with "quotes"', 'line\nbreak', 'tab\t', 'café', '😀', 'back\\slash', '<tag>&']);
        const item: any = { metadata: metadata(r, i, name, ns, r() > 0.6 ? { labels: { app: 'x', '1': 'one' } } : {}) };
        if (Object.keys(data).length) item.data = sortedGoOrder(data);
        if (r() > 0.8) item.binaryData = { 'blob.bin': 'AAEC', '5': '' };
        items.push(item);
    });
    items.push({ metadata: { name: 'kube-root-ca.crt', namespace: 'default', uid: 'uid-edge-1', creationTimestamp: iso(50) }, data: { 'ca.crt': 'x'.repeat(40) } });
    items.push({ metadata: { name: 'empty', namespace: 'default', uid: 'uid-edge-2', creationTimestamp: iso(51) } });
    items.push({ metadata: { name: 'no-timestamp', namespace: 'default', uid: 'uid-edge-3', creationTimestamp: null } });
    items.push({ metadata: { name: 'numeric-keys', namespace: 'prod', uid: 'uid-edge-4', creationTimestamp: iso(52) }, data: sortedGoOrder({ b: '1', '10': '2', '9': '3', a: '4', '007': '5', '4294967295': '6' }) });
    items.push({ metadata: { name: 'same-name', namespace: 'team-a', uid: 'uid-edge-5', creationTimestamp: iso(53) }, data: { k: 'v' } });
    items.push({ metadata: { name: 'same-name', namespace: 'team-b', uid: 'uid-edge-6', creationTimestamp: iso(53) }, data: { k: 'v' } });
    items.push(...tieGroup('prod', (_n, metadata) => ({ metadata, data: { k: 'v' } })));
    return { type: 'configmaps', items: canonical(items), noSort: ['uid-edge-3'] };
}

/** Go marshals maps with sorted keys (byte order); fixtures mirror that insertion order. */
function sortedGoOrder<T>(obj: Record<string, T>): Record<string, T> {
    const out: Record<string, T> = {};
    const enc = new TextEncoder();
    const byBytes = (a: string, b: string) => {
        const x = enc.encode(a), y = enc.encode(b);
        for (let i = 0; i < Math.min(x.length, y.length); i++) if (x[i] !== y[i]) return x[i] - y[i];
        return x.length - y.length;
    };
    for (const k of Object.keys(obj).sort(byBytes)) out[k] = obj[k];
    return out;
}

// Secret list items are SecretListItem: metadata, type (omitted when empty) and dataKeys.
export function secretFixtures(): FixtureSet {
    const r = prng(37);
    const types = ['Opaque', 'kubernetes.io/tls', 'kubernetes.io/dockerconfigjson', 'helm.sh/release.v1', 'kubernetes.io/service-account-token', 'bootstrap.kubernetes.io/token'];
    const items: any[] = [];
    NAMES.forEach((name, i) => {
        const ns = pick(r, NAMESPACES);
        const item: any = { metadata: metadata(r, i, name, ns, r() > 0.7 ? { labels: { owner: 'helm' } } : {}), type: pick(r, types), dataKeys: Math.floor(r() * 6) };
        items.push(item);
    });
    items.push({ metadata: { name: 'no-type', namespace: 'default', uid: 'uid-edge-1', creationTimestamp: iso(60) }, dataKeys: 0 });
    items.push({ metadata: { name: 'no-timestamp', namespace: 'default', uid: 'uid-edge-2', creationTimestamp: null }, type: 'Opaque', dataKeys: 2 });
    items.push({ metadata: { name: 'sh.helm.release.v1.app.v1', namespace: 'prod', uid: 'uid-edge-3', creationTimestamp: iso(61) }, type: 'helm.sh/release.v1', dataKeys: 1 });
    items.push({ metadata: { name: 'big', namespace: 'prod', uid: 'uid-edge-4', creationTimestamp: iso(62) }, type: 'Opaque', dataKeys: 12 });
    items.push(...tieGroup('prod', (_n, metadata) => ({ metadata, type: 'Opaque', dataKeys: 1 })));
    return { type: 'secrets', items: canonical(items), noSort: ['uid-edge-1', 'uid-edge-2'] };
}

export function eventFixtures(): FixtureSet {
    const r = prng(41);
    const items: any[] = [];
    const reasons = ['Scheduled', 'Pulled', 'Started', 'BackOff', 'FailedMount', 'Killing', 'Unhealthy'];
    const kinds = ['Pod', 'Deployment', 'Node', 'ReplicaSet', 'Service'];
    const messages = ['Successfully assigned', 'Back-off restarting failed container', 'Readiness probe failed: HTTP 503', 'MountVolume.SetUp failed for volume "cfg"', 'café message', ''];
    NAMES.forEach((name, i) => {
        const ns = pick(r, NAMESPACES);
        const first = iso(Math.floor(r() * 4000) + 100);
        const last = r() > 0.3 ? iso(Math.floor(r() * 90) + 1) : null;
        const item: any = {
            metadata: metadata(r, i, `${name}.${(i * 7919).toString(16)}`, ns),
            involvedObject: { kind: pick(r, kinds), name: pick(r, NAMES), namespace: ns, uid: `inv-${i}` },
            reason: pick(r, reasons),
            message: pick(r, messages),
            source: r() > 0.4 ? { component: pick(r, ['kubelet', 'default-scheduler', 'deployment-controller']), host: pick(r, ['node-1', 'node-2', '']) } : {},
            firstTimestamp: first,
            lastTimestamp: last,
            count: Math.floor(r() * 12),
            type: pick(r, ['Normal', 'Normal', 'Warning', 'Odd']),
        };
        if (item.count === 0) delete item.count;
        if (r() > 0.8) item.reportingComponent = 'kubernetes.io/kubelet';
        items.push(item);
    });
    items.push({ metadata: { name: 'no-first.1', namespace: 'default', uid: 'uid-edge-1', creationTimestamp: iso(70) }, involvedObject: {}, source: {}, firstTimestamp: null, lastTimestamp: null, type: 'Normal', reason: 'Edge', message: 'zero timestamps' });
    items.push({ metadata: { name: 'no-type.1', namespace: 'default', uid: 'uid-edge-2', creationTimestamp: iso(71) }, involvedObject: { kind: 'Pod', name: 'p' }, source: {}, firstTimestamp: iso(71), lastTimestamp: iso(71), reportingComponent: 'ctl' });
    items.push({ metadata: { name: 'no-ts.1', namespace: 'default', uid: 'uid-edge-3', creationTimestamp: null }, involvedObject: { kind: 'Pod', name: 'q' }, source: { component: 'kubelet' }, firstTimestamp: null, lastTimestamp: null, type: 'Warning', count: 1 });
    items.push({ metadata: { name: 'cluster.1', uid: 'uid-edge-4', creationTimestamp: iso(72) }, involvedObject: { kind: 'Node', name: 'n1' }, source: { component: 'kubelet', host: 'n1' }, firstTimestamp: iso(72), lastTimestamp: iso(2), type: 'Normal', count: 3 });
    items.push(...tieGroup('prod', (_n, metadata) => ({ metadata, involvedObject: { kind: 'Pod', name: 'p' }, source: { component: 'kubelet' }, firstTimestamp: iso(3000), lastTimestamp: iso(3000), type: 'Normal', reason: 'Tie', message: 'tie', count: 2 })));
    return { type: 'events', items: canonical(items), noSort: ['uid-edge-3', 'uid-edge-4'] };
}

/** Cluster-scoped fixtures: names from the shared pool plus edge cases, built by `build`. */
function clusterFixtures(type: string, seed: number, build: (r: () => number, name: string) => any, edges: any[], noSort: string[]): FixtureSet {
    const r = prng(seed);
    const items: any[] = [];
    NAMES.forEach((name, i) => {
        const item = build(r, name);
        const extraLabels = item.__labels;
        delete item.__labels;
        const labels = { ...(r() > 0.7 ? { tier: 'x', '10': 'ten' } : {}), ...(extraLabels ?? {}) };
        item.metadata = metadata(r, i, name, undefined, Object.keys(labels).length ? { labels } : {});
        items.push(item);
    });
    edges.forEach((e, k) => {
        delete e.__labels;
        e.metadata = { name: e.metadata?.name ?? `edge-${k}`, uid: `uid-edge-${k + 1}`, creationTimestamp: iso(80 + k), ...(e.metadata?.annotations ? { annotations: e.metadata.annotations } : {}), ...(e.metadata?.labels ? { labels: e.metadata.labels } : {}), ...(e.metadata?.creationTimestamp === null ? { creationTimestamp: null } : {}) };
        items.push(e);
    });
    items.push(...tieGroup(undefined, (_n, meta) => { const it = build(prng(1), 'tie'); delete it.__labels; return { ...it, metadata: meta }; }));
    return { type, items: canonical(items), noSort };
}

export function priorityClassFixtures(): FixtureSet {
    const policies = [undefined, 'PreemptLowerPriority', 'Never'];
    return clusterFixtures('priorityclasses', 53, (r, _name) => {
        const item: any = { value: pick(r, [0, 1, 10, 100, 1000, 1000000, 2000000000, -5, 999999999, 1000000000]) };
        if (r() > 0.7) item.globalDefault = true;
        const p = pick(r, policies);
        if (p) item.preemptionPolicy = p;
        if (r() > 0.5) item.description = pick(r, ['critical', 'batch work', 'café']);
        return item;
    }, [
        { value: 0, metadata: { name: 'zero' } },
        { value: 7, globalDefault: true, preemptionPolicy: 'Never', metadata: { name: 'default-never' } },
        { value: 7, metadata: { name: 'same-value-b' } },
        { value: 7, metadata: { name: 'same-value-a' } },
        { value: 3, metadata: { name: 'no-timestamp', creationTimestamp: null } },
    ], ['uid-edge-5']);
}

export function ingressClassFixtures(): FixtureSet {
    const controllers = ['k8s.io/ingress-nginx', 'traefik.io/ingress-controller', 'example.com/other', 'Example.com/Upper'];
    return clusterFixtures('ingressclasses', 59, (r, _name) => {
        const item: any = { spec: {} };
        if (r() > 0.2) item.spec.controller = pick(r, controllers);
        return item;
    }, [
        { spec: { controller: 'k8s.io/ingress-nginx' }, metadata: { name: 'default-class', annotations: { 'ingressclass.kubernetes.io/is-default-class': 'true' } } },
        { spec: { controller: 'x' }, metadata: { name: 'not-default', annotations: { 'ingressclass.kubernetes.io/is-default-class': 'false' } } },
        { spec: {}, metadata: { name: 'no-controller' } },
        { metadata: { name: 'no-spec' } },
        { spec: { controller: 'x' }, metadata: { name: 'no-timestamp', creationTimestamp: null } },
    ], ['uid-edge-5']);
}

export function storageClassFixtures(): FixtureSet {
    return clusterFixtures('storageclasses', 61, (r, _name) => {
        const item: any = { provisioner: pick(r, ['ebs.csi.aws.com', 'kubernetes.io/no-provisioner', 'rancher.io/local-path', 'disk.csi.azure.com', 'Zeta.io/csi']) };
        const rp = pick(r, [undefined, 'Delete', 'Retain', 'Recycle']);
        if (rp) item.reclaimPolicy = rp;
        const bm = pick(r, [undefined, 'Immediate', 'WaitForFirstConsumer']);
        if (bm) item.volumeBindingMode = bm;
        if (r() > 0.5) item.allowVolumeExpansion = r() > 0.3;
        return item;
    }, [
        { provisioner: 'p', reclaimPolicy: 'Delete', volumeBindingMode: 'Immediate', allowVolumeExpansion: true, metadata: { name: 'default-sc', annotations: { 'storageclass.kubernetes.io/is-default-class': 'true' } } },
        { provisioner: 'p', allowVolumeExpansion: false, metadata: { name: 'expansion-false' } },
        { provisioner: '', metadata: { name: 'empty-provisioner' } },
        { provisioner: 'p', metadata: { name: 'no-timestamp', creationTimestamp: null } },
    ], ['uid-edge-4']);
}

const webhookNames = ['a.example.com', 'b.example.com', 'c.example.com'];

function webhookEntry(r: () => number, mutating: boolean): any {
    const wh: any = { name: pick(r, webhookNames), clientConfig: {}, admissionReviewVersions: ['v1'] };
    if (r() > 0.3) wh.clientConfig.service = { namespace: pick(r, ['default', 'kube-system', 'cert-manager']), name: pick(r, ['webhook', 'svc-a', 'svc-b']), path: '/validate' };
    else wh.clientConfig.url = 'https://example.com/hook';
    const fp = pick(r, [undefined, 'Fail', 'Ignore', '']);
    if (fp !== undefined) wh.failurePolicy = fp;
    const se = pick(r, [undefined, 'None', 'NoneOnDryRun', 'Some']);
    if (se !== undefined) wh.sideEffects = se;
    if (mutating) {
        const rp = pick(r, [undefined, 'Never', 'IfNeeded']);
        if (rp !== undefined) wh.reinvocationPolicy = rp;
    }
    const nRules = Math.floor(r() * 3);
    if (nRules > 0) {
        wh.rules = [];
        for (let k = 0; k < nRules; k++) {
            wh.rules.push({
                operations: pick(r, [['CREATE'], ['CREATE', 'UPDATE'], ['*'], ['DELETE', 'CREATE']]),
                apiGroups: [''],
                apiVersions: ['v1'],
                resources: pick(r, [['pods'], ['deployments', 'statefulsets'], ['*'], ['pods', 'services']]),
            });
        }
    }
    return wh;
}

function webhookConfigFixtures(type: string, seed: number, mutating: boolean): FixtureSet {
    return clusterFixtures(type, seed, (r, _name) => {
        const item: any = {};
        const n = Math.floor(r() * 4);
        if (n > 0) item.webhooks = Array.from({ length: n }, () => webhookEntry(r, mutating));
        return item;
    }, [
        { metadata: { name: 'no-webhooks' } },
        { metadata: { name: 'dup-services' }, webhooks: [
            { name: 'one', clientConfig: { service: { namespace: 'ns', name: 'svc' } }, failurePolicy: 'Ignore', sideEffects: 'None', reinvocationPolicy: mutating ? 'IfNeeded' : undefined, rules: [{ operations: ['CREATE'], resources: ['pods'] }] },
            { name: 'two', clientConfig: { service: { namespace: 'ns', name: 'svc' } }, failurePolicy: 'Fail', sideEffects: 'Some', rules: [{ operations: ['CREATE', 'UPDATE'], resources: ['pods', 'jobs'] }, { operations: ['CREATE'], resources: ['pods'] }] },
            { name: 'three', clientConfig: { url: 'https://x' }, failurePolicy: '', rules: [] },
        ] },
        { metadata: { name: 'mixed-policies' }, webhooks: [
            { name: 'x', clientConfig: {}, failurePolicy: 'Ignore', reinvocationPolicy: mutating ? 'Never' : undefined },
            { name: 'y', clientConfig: {}, reinvocationPolicy: mutating ? 'IfNeeded' : undefined },
        ] },
        { metadata: { name: 'no-timestamp', creationTimestamp: null }, webhooks: [{ name: 'z', clientConfig: {} }] },
    ], ['uid-edge-4']);
}

export function validatingWebhookFixtures(): FixtureSet {
    return webhookConfigFixtures('validatingwebhookconfigurations', 67, false);
}

export function mutatingWebhookFixtures(): FixtureSet {
    return webhookConfigFixtures('mutatingwebhookconfigurations', 71, true);
}

export function csiDriverFixtures(): FixtureSet {
    return clusterFixtures('csidrivers', 73, (r, _name) => {
        const spec: any = {};
        const bool = () => pick(r, [undefined, true, false]);
        for (const k of ['attachRequired', 'podInfoOnMount', 'storageCapacity']) {
            const v = bool();
            if (v !== undefined) spec[k] = v;
        }
        const modes = pick(r, [undefined, ['Persistent'], ['Persistent', 'Ephemeral'], ['Ephemeral']]);
        if (modes) spec.volumeLifecycleModes = modes;
        const fsg = pick(r, [undefined, 'ReadWriteOnceWithFSType', 'File', 'None']);
        if (fsg) spec.fsGroupPolicy = fsg;
        return { spec };
    }, [
        { spec: {}, metadata: { name: 'all-defaults' } },
        { spec: { attachRequired: false, podInfoOnMount: true, storageCapacity: true, volumeLifecycleModes: ['Persistent', 'Ephemeral'] }, metadata: { name: 'all-set' } },
        { spec: { attachRequired: true, volumeLifecycleModes: [] }, metadata: { name: 'empty-modes' } },
        { spec: {}, metadata: { name: 'no-timestamp', creationTimestamp: null } },
    ], ['uid-edge-4']);
}

export function csiNodeFixtures(): FixtureSet {
    const driverNames = ['ebs.csi.aws.com', 'disk.csi.azure.com', 'Zeta.io/csi', 'csi.example.com'];
    return clusterFixtures('csinodes', 79, (r, _name) => {
        const n = Math.floor(r() * 5);
        const drivers = Array.from({ length: n }, (_x, k) => ({ name: pick(r, driverNames), nodeID: `node-${k}-${Math.floor(r() * 100)}`, topologyKeys: ['topology.kubernetes.io/zone'] }));
        return { spec: { drivers: n === 0 ? null : drivers } };
    }, [
        { spec: { drivers: null }, metadata: { name: 'null-drivers' } },
        { spec: { drivers: [{ name: 'one', nodeID: 'n1' }] }, metadata: { name: 'one-driver' } },
        { spec: { drivers: [{ name: 'one', nodeID: 'n1' }, { name: 'two', nodeID: 'n2' }] }, metadata: { name: 'two-drivers' } },
        { spec: { drivers: [{ name: 'one', nodeID: 'n1' }, { name: 'two', nodeID: 'n2' }, { name: 'three', nodeID: 'n3' }] }, metadata: { name: 'three-drivers' } },
        { spec: { drivers: [{ name: 'only', nodeID: '' }] }, metadata: { name: 'empty-node-id' } },
        { spec: { drivers: [{ name: 'one', nodeID: 'n1' }] }, metadata: { name: 'no-timestamp', creationTimestamp: null } },
    ], ['uid-edge-6']);
}

export function pvFixtures(): FixtureSet {
    const sizes = ['1Gi', '10Gi', '500Mi', '1Ti', '100G', '5Gi'];
    const modes = [['ReadWriteOnce'], ['ReadWriteOnce', 'ReadOnlyMany'], ['ReadWriteMany'], ['ReadWriteOncePod'], ['ReadOnlyMany', 'ReadWriteMany', 'ReadWriteOnce']];
    return clusterFixtures('persistentvolumes', 83, (r, _name) => {
        const spec: any = { capacity: { storage: pick(r, sizes) }, accessModes: pick(r, modes) };
        const rp = pick(r, [undefined, 'Delete', 'Retain', 'Recycle']);
        if (rp) spec.persistentVolumeReclaimPolicy = rp;
        if (r() > 0.4) spec.claimRef = { namespace: pick(r, ['default', 'prod', 'team-a']), name: pick(r, ['data-0', 'data-1', 'pvc-x']), kind: 'PersistentVolumeClaim' };
        const sc = pick(r, [undefined, 'standard', 'gp3', 'fast']);
        if (sc) spec.storageClassName = sc;
        return { spec, status: { phase: pick(r, ['Bound', 'Bound', 'Available', 'Released', 'Failed', 'Pending']) } };
    }, [
        { spec: { capacity: { storage: '1Gi' } }, status: { phase: 'Available' }, metadata: { name: 'minimal' } },
        { spec: {}, status: { phase: 'Pending' }, metadata: { name: 'no-capacity' } },
        { spec: { capacity: { storage: '1Gi' }, accessModes: [] }, status: { phase: 'Bound' }, metadata: { name: 'empty-modes' } },
        { spec: { capacity: { storage: '2Gi' }, accessModes: ['ReadWriteOnce'], claimRef: { namespace: 'a', name: 'b' } }, status: {}, metadata: { name: 'no-phase' } },
        { spec: { capacity: { storage: '2Gi' } }, status: { phase: 'Bound' }, metadata: { name: 'no-timestamp', creationTimestamp: null } },
    ], ['uid-edge-4', 'uid-edge-5']);
}

const rbacRules = [
    { apiGroups: [''], resources: ['pods', 'services'], verbs: ['get', 'list', 'watch'] },
    { apiGroups: ['apps'], resources: ['deployments'], verbs: ['*'] },
    { apiGroups: ['', 'batch'], resources: ['jobs', 'pods'], verbs: ['create', 'delete'] },
    { resources: ['secrets'], verbs: ['get'] },
    { nonResourceURLs: ['/healthz'], verbs: ['get'] },
    { apiGroups: ['rbac.authorization.k8s.io'], resources: ['clusterroles'], verbs: ['bind', 'get'], resourceNames: ['admin'] },
];

export function clusterRoleFixtures(): FixtureSet {
    return clusterFixtures('clusterroles', 89, (r, _name) => {
        const item: any = {};
        const n = Math.floor(r() * 4);
        if (n > 0) item.rules = Array.from({ length: n }, () => pick(r, rbacRules));
        if (r() > 0.8) item.aggregationRule = { clusterRoleSelectors: [{ matchLabels: { 'rbac.example.com/aggregate-to-admin': 'true' } }] };
        return item;
    }, [
        { metadata: { name: 'no-rules' } },
        { rules: [rbacRules[0], rbacRules[0], rbacRules[2]], metadata: { name: 'dup-rules' } },
        { aggregationRule: { clusterRoleSelectors: [{ matchLabels: { a: '1', b: '2' } }, { matchLabels: { c: '3' } }] }, metadata: { name: 'aggregated' } },
        { aggregationRule: { clusterRoleSelectors: [] }, rules: [rbacRules[1]], metadata: { name: 'empty-selectors' } },
        { aggregationRule: { clusterRoleSelectors: [{ matchExpressions: [{ key: 'k', operator: 'Exists' }] }] }, metadata: { name: 'expression-selector' } },
        { rules: [rbacRules[3]], metadata: { name: 'no-timestamp', creationTimestamp: null } },
    ], ['uid-edge-6']);
}

const subjectKinds = ['User', 'Group', 'ServiceAccount'];

export function clusterRoleBindingFixtures(): FixtureSet {
    return clusterFixtures('clusterrolebindings', 97, (r, _name) => {
        const item: any = { roleRef: { apiGroup: 'rbac.authorization.k8s.io', kind: pick(r, ['ClusterRole', 'Role']), name: pick(r, ['admin', 'view', 'edit', 'cluster-admin', 'Zeta']) } };
        const n = Math.floor(r() * 4);
        if (n > 0) {
            item.subjects = Array.from({ length: n }, () => {
                const kind = pick(r, subjectKinds);
                const s: any = { kind, name: pick(r, ['alice', 'bob', 'devs', 'default', 'system:masters']) };
                if (kind === 'ServiceAccount') s.namespace = pick(r, ['default', 'kube-system', 'prod']);
                else s.apiGroup = 'rbac.authorization.k8s.io';
                return s;
            });
        }
        return item;
    }, [
        { roleRef: { apiGroup: 'rbac.authorization.k8s.io', kind: 'ClusterRole', name: 'cluster-admin' }, metadata: { name: 'no-subjects' } },
        { roleRef: { kind: '', name: '' }, subjects: [{ kind: 'User', name: 'carol' }], metadata: { name: 'empty-ref' } },
        { roleRef: { kind: 'ClusterRole', name: 'view' }, subjects: [{ kind: 'ServiceAccount', name: 'sa', namespace: 'a' }, { kind: 'ServiceAccount', name: 'sb' }, { kind: 'User', name: 'sa' }, { kind: 'User', name: 'sa' }], metadata: { name: 'mixed-subjects' } },
        { roleRef: { kind: 'ClusterRole', name: 'admin' }, subjects: [{ kind: 'Group', name: 'g1' }], metadata: { name: 'no-timestamp', creationTimestamp: null } },
    ], ['uid-edge-4']);
}

// ---------------------------------------------------------------------------------------------
// Namespaced configuration and policy types.

type NamespacedEdge = any & { __noSort?: boolean };

/** Random items carry their owner references as __owners; they belong in metadata, which the builder rewrites. */
function takeOwners(item: any): any[] | undefined {
    const owners = item.__owners;
    delete item.__owners;
    delete item.__meta;
    return owners;
}

/**
 * Namespaced fixtures: names from the shared pool spread over namespaces plus edge cases built by
 * `build`. An edge may carry `__noSort` to be excluded from the sort goldens (mixed value types in
 * one column are ordered by an inconsistent legacy comparator, which no engine can reproduce).
 */
function namespacedFixtures(type: string, seed: number, build: (r: () => number, name: string) => any, edges: NamespacedEdge[]): FixtureSet {
    const r = prng(seed);
    const items: any[] = [];
    NAMES.forEach((name, i) => {
        const item = build(r, name);
        const metaExtra = item.__meta;
        const owners = takeOwners(item);
        item.metadata = metadata(r, i, name, pick(r, NAMESPACES), { ...(r() > 0.7 ? { labels: { tier: 'x', '10': 'ten' } } : {}), ...(owners ? { ownerReferences: owners } : {}), ...(metaExtra ?? {}) });
        items.push(item);
    });
    const noSort: string[] = [];
    edges.forEach((e, k) => {
        const { __noSort, ...item } = e;
        const m = item.metadata ?? {};
        const uid = `uid-edge-${k + 1}`;
        item.metadata = {
            name: m.name ?? `edge-${k}`, namespace: m.namespace ?? 'default', uid, creationTimestamp: iso(80 + k),
            ...(m.annotations ? { annotations: m.annotations } : {}),
            ...(m.labels ? { labels: m.labels } : {}),
            ...(m.ownerReferences ? { ownerReferences: m.ownerReferences } : {}),
            ...(m.deletionTimestamp ? { deletionTimestamp: m.deletionTimestamp } : {}),
            ...(m.creationTimestamp === null ? { creationTimestamp: null } : {}),
        };
        if (__noSort || m.creationTimestamp === null) noSort.push(uid);
        items.push(item);
    });
    items.push(...tieGroup('prod', (_n, meta) => { const it = build(prng(1), 'tie'); takeOwners(it); return { ...it, metadata: meta }; }));
    return { type, items: canonical(items), noSort };
}

const annotatedEdge = (extra: any) => ({ ...extra, metadata: { name: 'annotated', annotations: { note: 'Hello World', 'a/b': 'c=d' } } });
const sameNameEdges = (extra: any) => [
    { ...extra, metadata: { name: 'same-name', namespace: 'team-a' } },
    { ...extra, metadata: { name: 'same-name', namespace: 'team-b' } },
];
const noTimestampEdge = (extra: any) => ({ ...extra, metadata: { name: 'no-timestamp', creationTimestamp: null } });

const hpaTargets: Array<[string, string]> = [['Deployment', 'web'], ['StatefulSet', 'db'], ['ReplicaSet', 'rs-1'], ['Deployment', 'Zeta'], ['Rollout', 'canary']];
const hpaMetrics = [
    { type: 'Resource', resource: { name: 'cpu', target: { type: 'Utilization', averageUtilization: 80 } } },
    { type: 'Pods', pods: { metric: { name: 'qps' }, target: { type: 'AverageValue', averageValue: '100' } } },
    { type: 'External', external: { metric: { name: 'queue' }, target: { type: 'Value', value: '30' } } },
    { type: 'ContainerResource', containerResource: { name: 'memory', container: 'app', target: { type: 'Utilization', averageUtilization: 70 } } },
];

function hpaItem(kind: string, name: string, max: number, min: number | undefined, desired: number, current: number | undefined, metrics?: any[]) {
    const spec: any = { scaleTargetRef: { kind, name, apiVersion: 'apps/v1' }, maxReplicas: max };
    if (min !== undefined) spec.minReplicas = min;
    if (metrics) spec.metrics = metrics;
    const status: any = { desiredReplicas: desired };
    if (current !== undefined) status.currentReplicas = current;
    return { spec, status };
}

export function hpaFixtures(): FixtureSet {
    return namespacedFixtures('hpas', 101, (r, _name) => {
        const [kind, name] = pick(r, hpaTargets);
        const nm = Math.floor(r() * 3);
        const metrics = nm > 0 ? Array.from({ length: nm }, () => pick(r, hpaMetrics)) : undefined;
        return hpaItem(kind, name, pick(r, [1, 3, 5, 10, 20, 100]), pick(r, [undefined, 1, 2, 3, 5]), Math.floor(r() * 8) + 1, Math.floor(r() * 8) + 1, metrics);
    }, [
        { ...hpaItem('Deployment', 'a', 3, undefined, 1, 1), metadata: { name: 'no-min' } },
        { ...hpaItem('Deployment', 'a', 4, 0, 1, 2), metadata: { name: 'min-zero' } },
        { ...hpaItem('Deployment', 'a', 0, 2, 1, 1), metadata: { name: 'max-zero' } },
        { ...hpaItem('Deployment', 'a', 5, 1, 2, undefined), metadata: { name: 'no-current' }, __noSort: true },
        { ...hpaItem('', '', 5, 1, 1, 1), metadata: { name: 'empty-target' } },
        { ...hpaItem('Deployment', 'a', 10, 2, 0, 12), metadata: { name: 'desired-zero' } },
        annotatedEdge(hpaItem('Deployment', 'a', 3, 1, 1, 1, [hpaMetrics[0], hpaMetrics[0], hpaMetrics[2]])),
        ...sameNameEdges(hpaItem('Deployment', 'same', 3, 1, 1, 1)),
        noTimestampEdge(hpaItem('Deployment', 'a', 3, 1, 1, 1)),
    ]);
}

const pdbSelectors: Array<Record<string, string>> = [{ app: 'web' }, { app: 'db', tier: 'data' }, { '10': 'ten', '9': 'nine', app: 'x' }, { 'app.kubernetes.io/name': 'cache' }];

function pdbItem(spec: any, status: any = { disruptionsAllowed: 1, currentHealthy: 3, desiredHealthy: 2, expectedPods: 3 }) {
    return { spec, status };
}

export function pdbFixtures(): FixtureSet {
    return namespacedFixtures('pdbs', 103, (r, _name) => {
        const spec: any = { minAvailable: pick(r, [1, 2, 3, 5, 10]) };
        if (r() > 0.2) spec.selector = { matchLabels: sortedGoOrder(pick(r, pdbSelectors)) };
        return pdbItem(spec, {
            disruptionsAllowed: Math.floor(r() * 4), currentHealthy: Math.floor(r() * 6), desiredHealthy: Math.floor(r() * 6), expectedPods: Math.floor(r() * 8),
        });
    }, [
        { ...pdbItem({ minAvailable: 0 }), metadata: { name: 'min-zero' } },
        { ...pdbItem({ minAvailable: 1 }, { disruptionsAllowed: 0, currentHealthy: 0, desiredHealthy: 0, expectedPods: 0 }), metadata: { name: 'status-zero' } },
        { ...pdbItem({ maxUnavailable: 1, selector: { matchLabels: { app: 'x' } } }), metadata: { name: 'max-int' }, __noSort: true },
        { ...pdbItem({ minAvailable: '50%' }), metadata: { name: 'min-percent' }, __noSort: true },
        { ...pdbItem({ maxUnavailable: '25%', selector: { matchLabels: sortedGoOrder({ b: '2', a: '1' }) } }), metadata: { name: 'max-percent' }, __noSort: true },
        { ...pdbItem({ minAvailable: '' }), metadata: { name: 'min-empty-string' }, __noSort: true },
        { ...pdbItem({ minAvailable: '2' }), metadata: { name: 'min-numeric-string' }, __noSort: true },
        { ...pdbItem({}), metadata: { name: 'no-budget' }, __noSort: true },
        annotatedEdge(pdbItem({ minAvailable: 2, selector: { matchLabels: { app: 'a' } } })),
        ...sameNameEdges(pdbItem({ minAvailable: 1 })),
        noTimestampEdge(pdbItem({ minAvailable: 1 })),
    ]);
}

const quotaPool: Record<string, string[]> = {
    cpu: ['1', '2', '4', '10', '500m'],
    memory: ['1Gi', '4Gi', '512Mi', '10Gi'],
    pods: ['10', '50', '100'],
    'requests.cpu': ['1', '2'],
    'limits.memory': ['8Gi'],
    'count/deployments.apps': ['5'],
    'services.loadbalancers': ['0', '1'],
};

function quotaItem(hard: Record<string, string>, used: Record<string, string> | undefined, scopes?: string[]) {
    const spec: any = {};
    if (Object.keys(hard).length) spec.hard = sortedGoOrder(hard);
    if (scopes) spec.scopes = scopes;
    const status: any = {};
    if (Object.keys(hard).length) status.hard = sortedGoOrder(hard);
    if (used && Object.keys(used).length) status.used = sortedGoOrder(used);
    return { spec, status };
}

export function resourceQuotaFixtures(): FixtureSet {
    return namespacedFixtures('resourcequotas', 107, (r, _name) => {
        const keys = Object.keys(quotaPool).filter(() => r() > 0.6);
        const hard: Record<string, string> = {};
        const used: Record<string, string> = {};
        for (const k of keys) {
            hard[k] = pick(r, quotaPool[k]);
            if (r() > 0.3) used[k] = pick(r, quotaPool[k]);
        }
        return quotaItem(hard, used, r() > 0.8 ? pick(r, [['BestEffort'], ['NotTerminating', 'NotBestEffort']]) : undefined);
    }, [
        { ...quotaItem({}, undefined), metadata: { name: 'no-hard' } },
        { ...quotaItem({ cpu: '2', memory: '4Gi', pods: '10' }, { cpu: '500m', memory: '1Gi', pods: '3' }), metadata: { name: 'three-resources' } },
        { ...quotaItem({ pods: '10' }, { pods: '10' }, ['Terminating']), metadata: { name: 'scoped' } },
        { ...quotaItem({ cpu: '1' }, undefined, ['BestEffort', 'NotTerminating', 'PriorityClass']), metadata: { name: 'multi-scope' } },
        annotatedEdge(quotaItem({ 'requests.cpu': '1' }, { 'requests.cpu': '1' })),
        ...sameNameEdges(quotaItem({ cpu: '1' }, { cpu: '1' })),
        noTimestampEdge(quotaItem({ cpu: '1' }, undefined)),
    ]);
}

const limitTemplates: any[] = [
    { type: 'Container', max: { cpu: '2' }, min: { cpu: '100m' }, default: { cpu: '500m' }, defaultRequest: { cpu: '200m' } },
    { type: 'Container', max: { memory: '4Gi' }, default: { memory: '512Mi' } },
    { type: 'Pod', max: { cpu: '4', memory: '8Gi' } },
    { type: 'PersistentVolumeClaim', max: { storage: '10Gi' }, min: { storage: '1Gi' } },
];

export function limitRangeFixtures(): FixtureSet {
    return namespacedFixtures('limitranges', 109, (r, _name) => {
        const n = Math.floor(r() * 4);
        return { spec: { limits: Array.from({ length: n }, () => pick(r, limitTemplates)) } };
    }, [
        { spec: { limits: [] }, metadata: { name: 'empty-limits' } },
        { spec: { limits: null }, metadata: { name: 'null-limits' } },
        { spec: { limits: [limitTemplates[0], limitTemplates[1]] }, metadata: { name: 'two-containers' } },
        { spec: { limits: [limitTemplates[3]] }, metadata: { name: 'pvc-only' } },
        { spec: { limits: [limitTemplates[0], limitTemplates[2], limitTemplates[3]] }, metadata: { name: 'all-kinds' } },
        annotatedEdge({ spec: { limits: [limitTemplates[2]] } }),
        ...sameNameEdges({ spec: { limits: [limitTemplates[0]] } }),
        noTimestampEdge({ spec: { limits: [limitTemplates[0]] } }),
    ]);
}

/** metav1.MicroTime as the API serialises it: exactly six fractional digits. */
const micro = (offsetMs: number) => new Date(GOLDEN_NOW - offsetMs).toISOString().replace(/Z$/, '000Z');

function leaseItem(spec: any) {
    return { spec };
}

export function leaseFixtures(): FixtureSet {
    const holders = ['node-1_5f1e9c', 'kube-controller-manager-abc_1234', '', 'Zeta', 'café-holder', '10', '9', 'ip-10-0-0-1.ec2.internal_8b9a'];
    return namespacedFixtures('leases', 113, (r, _name) => {
        const spec: any = {};
        const h = pick(r, [...holders, undefined]);
        if (h !== undefined) spec.holderIdentity = h;
        const d = pick(r, [undefined, 15, 30, 60, 137, 1]);
        if (d !== undefined) spec.leaseDurationSeconds = d;
        if (r() > 0.2) spec.renewTime = micro(Math.floor(r() * 3600000) + 1000);
        if (r() > 0.5) spec.acquireTime = micro(Math.floor(r() * 36000000) + 3600000);
        const t = pick(r, [undefined, 0, 1, 5, 12]);
        if (t !== undefined) spec.leaseTransitions = t;
        return leaseItem(spec);
    }, [
        { ...leaseItem({}), metadata: { name: 'empty-spec' } },
        { ...leaseItem({ holderIdentity: 'a', leaseDurationSeconds: 0, renewTime: micro(1500), leaseTransitions: 0 }), metadata: { name: 'zeros' } },
        { ...leaseItem({ holderIdentity: 'a', leaseDurationSeconds: 15, renewTime: null }), metadata: { name: 'null-renew' } },
        { ...leaseItem({ holderIdentity: '', leaseDurationSeconds: 15, renewTime: micro(2500) }), metadata: { name: 'empty-holder' } },
        { ...leaseItem({ holderIdentity: 'same-renew', leaseDurationSeconds: 15, renewTime: micro(4000) }), metadata: { name: 'renew-tie-a' } },
        { ...leaseItem({ holderIdentity: 'same-renew', leaseDurationSeconds: 15, renewTime: micro(4000) }), metadata: { name: 'renew-tie-b' } },
        annotatedEdge(leaseItem({ holderIdentity: 'x', leaseDurationSeconds: 30, renewTime: micro(100) })),
        ...sameNameEdges(leaseItem({ holderIdentity: 'x', leaseDurationSeconds: 30, renewTime: micro(100) })),
        noTimestampEdge(leaseItem({ holderIdentity: 'x', leaseDurationSeconds: 30, renewTime: micro(100) })),
    ]);
}

const npLabelSets: Array<Record<string, string> | undefined> = [
    undefined, { app: 'web' }, { app: 'web', tier: 'frontend' }, { app: 'web', env: 'prod', tier: 'x' },
    { '10': 'ten', '9': 'nine', Zed: 'z', app: 'w' }, { a: '1', b: '2', c: '3', d: '4' },
];

function npItem(matchLabels: Record<string, string> | undefined, policyTypes?: string[], ingress?: any[], egress?: any[], matchExpressions?: any[]) {
    const podSelector: any = {};
    if (matchLabels) podSelector.matchLabels = sortedGoOrder(matchLabels);
    if (matchExpressions) podSelector.matchExpressions = matchExpressions;
    const spec: any = { podSelector };
    if (ingress) spec.ingress = ingress;
    if (egress) spec.egress = egress;
    if (policyTypes) spec.policyTypes = policyTypes;
    return { spec };
}

export function networkPolicyFixtures(): FixtureSet {
    return namespacedFixtures('networkpolicies', 127, (r, _name) => npItem(
        pick(r, npLabelSets),
        pick(r, [undefined, ['Ingress'], ['Egress'], ['Ingress', 'Egress']]),
        pick(r, [undefined, [{}], [{ from: [{ podSelector: {} }] }, {}]]),
        pick(r, [undefined, [{}], [{ to: [{ ipBlock: { cidr: '10.0.0.0/8' } }] }]]),
    ), [
        { ...npItem(undefined, undefined, undefined, undefined, [{ key: 'app', operator: 'Exists' }]), metadata: { name: 'expressions-only' } },
        { ...npItem({ app: 'a' }, ['Egress'], undefined, [{}, {}, {}]), metadata: { name: 'egress-three' } },
        { ...npItem({ a: '1', b: '2' }, ['Ingress', 'Egress'], [{}], [{}]), metadata: { name: 'exactly-two' } },
        { ...npItem({ a: '1', b: '2', c: '3' }, undefined, [{}], undefined), metadata: { name: 'exactly-three' } },
        { ...npItem({ '1': 'one', '2': 'two', '3': 'three', x: 'y' }, undefined), metadata: { name: 'numeric-labels' } },
        annotatedEdge(npItem({ app: 'a' }, ['Ingress'])),
        ...sameNameEdges(npItem({ app: 'a' }, ['Ingress'])),
        noTimestampEdge(npItem(undefined, ['Ingress'])),
    ]);
}

function saItem(secrets?: string[], pull?: string[], automount?: boolean) {
    const item: any = {};
    if (secrets) item.secrets = secrets.map(name => ({ name }));
    if (pull) item.imagePullSecrets = pull.map(name => ({ name }));
    if (automount !== undefined) item.automountServiceAccountToken = automount;
    return item;
}

export function serviceAccountFixtures(): FixtureSet {
    return namespacedFixtures('serviceaccounts', 131, (r, _name) => saItem(
        pick(r, [undefined, ['sa-token-abc12'], ['sa-token-a', 'dockercfg-b'], ['a', 'b', 'c']]),
        pick(r, [undefined, ['regcred'], ['r1', 'r2']]),
        pick(r, [undefined, true, false]),
    ), [
        { ...saItem(), metadata: { name: 'bare' } },
        { ...saItem(['t1', 't2', 't3', 't4', 't5']), metadata: { name: 'five-secrets' } },
        { ...saItem(undefined, ['pull-only'], false), metadata: { name: 'pull-only' } },
        { ...saItem(['dup', 'dup'], ['dup'], true), metadata: { name: 'dups' } },
        annotatedEdge(saItem(['t'])),
        ...sameNameEdges(saItem(['t'])),
        noTimestampEdge(saItem(['t'])),
    ]);
}

// ---------------------------------------------------------------------------------------------
// Namespaced network and RBAC types.

export function roleFixtures(): FixtureSet {
    return namespacedFixtures('roles', 137, (r, _name) => {
        const item: any = {};
        const n = Math.floor(r() * 4);
        if (n > 0) item.rules = Array.from({ length: n }, () => pick(r, rbacRules));
        return item;
    }, [
        { metadata: { name: 'no-rules' } },
        { rules: [rbacRules[0], rbacRules[0], rbacRules[2]], metadata: { name: 'dup-rules' } },
        { rules: [rbacRules[3], rbacRules[4], rbacRules[5]], metadata: { name: 'odd-rules' } },
        annotatedEdge({ rules: [rbacRules[1]] }),
        ...sameNameEdges({ rules: [rbacRules[0]] }),
        noTimestampEdge({ rules: [rbacRules[3]] }),
    ]);
}

export function roleBindingFixtures(): FixtureSet {
    return namespacedFixtures('rolebindings', 139, (r, _name) => {
        const item: any = { roleRef: { apiGroup: 'rbac.authorization.k8s.io', kind: pick(r, ['Role', 'ClusterRole']), name: pick(r, ['admin', 'view', 'edit', 'cluster-admin', 'Zeta']) } };
        const n = Math.floor(r() * 4);
        if (n > 0) {
            item.subjects = Array.from({ length: n }, () => {
                const kind = pick(r, subjectKinds);
                const s: any = { kind, name: pick(r, ['alice', 'bob', 'devs', 'default', 'system:masters']) };
                if (kind === 'ServiceAccount') s.namespace = pick(r, ['default', 'kube-system', 'prod']);
                else s.apiGroup = 'rbac.authorization.k8s.io';
                return s;
            });
        }
        return item;
    }, [
        { roleRef: { apiGroup: 'rbac.authorization.k8s.io', kind: 'ClusterRole', name: 'cluster-admin' }, metadata: { name: 'no-subjects' } },
        { roleRef: { kind: '', name: '' }, subjects: [{ kind: 'User', name: 'carol' }], metadata: { name: 'empty-ref' } },
        { roleRef: { kind: 'Role', name: 'view' }, subjects: [{ kind: 'ServiceAccount', name: 'sa', namespace: 'a' }, { kind: 'ServiceAccount', name: 'sb' }, { kind: 'User', name: 'sa' }, { kind: 'User', name: 'sa' }], metadata: { name: 'mixed-subjects' } },
        annotatedEdge({ roleRef: { kind: 'Role', name: 'admin' }, subjects: [{ kind: 'Group', name: 'g1' }] }),
        ...sameNameEdges({ roleRef: { kind: 'Role', name: 'admin' }, subjects: [{ kind: 'Group', name: 'g1' }] }),
        noTimestampEdge({ roleRef: { kind: 'ClusterRole', name: 'admin' }, subjects: [{ kind: 'Group', name: 'g1' }] }),
    ]);
}

const epPorts: any[][] = [
    [{ port: 80, protocol: 'TCP' }],
    [{ port: 443, protocol: 'TCP' }, { port: 8080 }],
    [{ port: 53, protocol: 'UDP' }, { port: 53, protocol: 'TCP' }],
    [{ port: 9000, protocol: 'TCP' }, { port: 9001, protocol: 'TCP' }, { port: 9002, protocol: 'TCP' }, { port: 9003, protocol: 'TCP' }],
    [],
];

function epAddress(r: () => number, i: number, withRef: boolean) {
    const a: any = { ip: `10.${Math.floor(r() * 4)}.${Math.floor(r() * 4)}.${i + 1}` };
    if (r() > 0.5) a.nodeName = pick(r, ['node-1', 'node-2']);
    if (withRef) a.targetRef = { kind: pick(r, ['Pod', 'Pod', 'Node']), namespace: 'default', name: pick(r, ['web-1', 'web-2', 'db-0']) };
    return a;
}

function epItem(subsets?: any[]) {
    const item: any = {};
    if (subsets) item.subsets = subsets;
    return item;
}

export function endpointsFixtures(): FixtureSet {
    return namespacedFixtures('endpoints', 149, (r, _name) => {
        const ns = Math.floor(r() * 3);
        if (ns === 0) return epItem(r() > 0.5 ? [] : undefined);
        const subsets = Array.from({ length: ns }, () => {
            const s: any = {};
            const ready = Math.floor(r() * 4);
            const notReady = Math.floor(r() * 3);
            if (ready > 0) s.addresses = Array.from({ length: ready }, (_, i) => epAddress(r, i, r() > 0.3));
            if (notReady > 0) s.notReadyAddresses = Array.from({ length: notReady }, (_, i) => epAddress(r, i + 10, r() > 0.5));
            const ports = pick(r, epPorts);
            if (ports.length > 0) s.ports = ports;
            return s;
        });
        return epItem(subsets);
    }, [
        { ...epItem(), metadata: { name: 'no-subsets' } },
        { ...epItem([]), metadata: { name: 'empty-subsets' } },
        { ...epItem([{ addresses: [{ ip: '10.9.9.1' }], ports: [{ port: 80, protocol: 'TCP' }] }]), metadata: { name: 'one-ready' } },
        { ...epItem([{ notReadyAddresses: [{ ip: '10.9.9.2' }, { ip: '10.9.9.3' }], ports: [{ port: 80 }] }]), metadata: { name: 'only-not-ready' } },
        { ...epItem([{ addresses: [{ ip: '10.9.9.4', targetRef: { kind: 'Pod', name: 'p' } }], notReadyAddresses: [{ ip: '10.9.9.5', targetRef: { kind: 'Pod', name: 'p' } }] }]), metadata: { name: 'dup-target' } },
        { ...epItem([{ addresses: [{ ip: '10.9.9.6' }], ports: [{ port: 80 }, { port: 80, protocol: 'TCP' }, { port: 81 }] }, { addresses: [{ ip: '10.9.9.7' }], ports: [{ port: 81, protocol: 'TCP' }, { port: 82 }, { port: 83 }, { port: 84 }] }]), metadata: { name: 'port-overflow' } },
        { ...epItem([{ ports: [{ port: 1, protocol: 'SCTP' }, { port: 2, name: 'named', protocol: 'UDP' }] }]), metadata: { name: 'ports-only' } },
        annotatedEdge(epItem([{ addresses: [{ ip: '10.9.9.8' }] }])),
        ...sameNameEdges(epItem([{ addresses: [{ ip: '10.9.9.9' }] }])),
        noTimestampEdge(epItem([{ addresses: [{ ip: '10.9.9.10' }] }])),
    ]);
}

function sliceItem(addressType: string | undefined, endpoints?: any[], ports?: any[], serviceName?: string) {
    const item: any = {};
    if (addressType !== undefined) item.addressType = addressType;
    if (endpoints) item.endpoints = endpoints;
    if (ports) item.ports = ports;
    if (serviceName !== undefined) item.__service = serviceName;
    return item;
}

const sliceEndpoint = (ready?: boolean | null) => {
    const e: any = { addresses: ['10.1.1.1'] };
    if (ready !== undefined) e.conditions = ready === null ? {} : { ready };
    return e;
};

const slicePorts: any[][] = [
    [{ port: 80, protocol: 'TCP' }],
    [{ name: 'https', port: 443, protocol: 'TCP' }, { name: 'metrics', port: 9090 }],
    [{ port: 53, protocol: 'UDP' }, { port: 53, protocol: 'TCP' }],
    [{ port: 1, protocol: 'TCP' }, { port: 2, protocol: 'TCP' }, { port: 3, protocol: 'TCP' }, { port: 4, protocol: 'TCP' }],
    [],
];

export function endpointSliceFixtures(): FixtureSet {
    const set = namespacedFixtures('endpointslices', 151, (r, _name) => {
        const n = Math.floor(r() * 5);
        const endpoints = n > 0 ? Array.from({ length: n }, () => sliceEndpoint(pick(r, [true, true, false, null, undefined]))) : undefined;
        const ports = pick(r, slicePorts);
        return sliceItem(pick(r, ['IPv4', 'IPv4', 'IPv6', 'FQDN']), endpoints, ports.length ? ports : undefined, pick(r, [undefined, 'web', 'Zeta', 'db']));
    }, [
        { ...sliceItem('IPv4'), metadata: { name: 'bare' } },
        { ...sliceItem(''), metadata: { name: 'empty-type' } },
        { ...sliceItem('IPv4', [sliceEndpoint(true), sliceEndpoint(true), sliceEndpoint(false)], [{ port: 80 }]), metadata: { name: 'two-ready' } },
        { ...sliceItem('IPv6', [sliceEndpoint(null), sliceEndpoint(undefined)], [{ name: 'noport' }, { name: 'x', protocol: '' }]), metadata: { name: 'no-conditions' } },
        { ...sliceItem('IPv4', [sliceEndpoint(true)], slicePorts[3]), metadata: { name: 'port-overflow' } },
        { ...sliceItem('IPv4', [sliceEndpoint(true)], [{ port: 80, protocol: 'TCP' }, { port: 80, protocol: 'TCP' }]), metadata: { name: 'dup-ports' } },
        annotatedEdge(sliceItem('IPv4', [sliceEndpoint(true)], [{ port: 80 }])),
        ...sameNameEdges(sliceItem('IPv4', [sliceEndpoint(true)], [{ port: 80 }])),
        noTimestampEdge(sliceItem('IPv4', [sliceEndpoint(true)], [{ port: 80 }])),
    ]);
    // The service name rides on the well-known label, as the API serves it.
    for (const item of set.items) {
        if (item.__service !== undefined) {
            item.metadata.labels = { ...(item.metadata.labels ?? {}), 'kubernetes.io/service-name': item.__service };
            item.metadata.labels = sortedGoOrder(item.metadata.labels);
        }
        delete item.__service;
    }
    return set;
}

// Services as the API serves them: targetPort always present (an int-or-string struct is never
// omitted), type and clusterIP set unless an edge case says otherwise.
function svcItem(opts: {
    type?: string; clusterIP?: string; ports?: any[]; selector?: Record<string, string>; affinity?: string;
    externalName?: string; externalIPs?: string[]; lb?: any[];
}) {
    const spec: any = {};
    if (opts.clusterIP !== undefined) spec.clusterIP = opts.clusterIP;
    if (opts.externalIPs) spec.externalIPs = opts.externalIPs;
    if (opts.externalName !== undefined) spec.externalName = opts.externalName;
    if (opts.ports) spec.ports = opts.ports;
    if (opts.selector) spec.selector = sortedGoOrder(opts.selector);
    if (opts.affinity !== undefined) spec.sessionAffinity = opts.affinity;
    if (opts.type !== undefined) spec.type = opts.type;
    const item: any = { spec };
    item.status = opts.lb ? { loadBalancer: { ingress: opts.lb } } : { loadBalancer: {} };
    return item;
}

const svcPortSets: any[][] = [
    [{ name: 'http', port: 80, protocol: 'TCP', targetPort: 8080 }],
    [{ name: 'https', port: 443, protocol: 'TCP', targetPort: 'https' }, { name: 'http', port: 80, protocol: 'TCP', targetPort: 8080, nodePort: 30080 }],
    [{ port: 53, protocol: 'UDP', targetPort: 53 }, { port: 53, protocol: 'TCP', targetPort: 53 }],
    [{ port: 5432, protocol: 'TCP', targetPort: 5432, nodePort: 31000 }],
];

export function serviceFixtures(): FixtureSet {
    return namespacedFixtures('services', 157, (r, _name) => {
        // ExternalName services have no cluster IP (an undefined sort key), so they only appear as noSort edges.
        const type = pick(r, ['ClusterIP', 'ClusterIP', 'NodePort', 'LoadBalancer']);
        const lb = type === 'LoadBalancer'
            ? pick(r, [undefined, [{ ip: '34.1.2.3' }], [{ hostname: 'elb.example.com' }], [{ ip: '34.1.2.4' }, { hostname: 'b.example.com' }], [{ hostname: 'z.example.com', ip: '' }]])
            : undefined;
        return svcItem({
            type,
            clusterIP: pick(r, ['10.96.0.1', '10.96.1.20', '10.96.0.9', 'None']),
            ports: pick(r, svcPortSets),
            selector: pick<Record<string, string> | undefined>(r, [undefined, { app: 'web' }, { app: 'web', tier: 'frontend' }, { '10': 'ten', '9': 'nine', app: 'x' }]),
            affinity: pick(r, [undefined, 'None', 'ClientIP']),
                        externalIPs: r() > 0.9 ? ['203.0.113.7'] : undefined,
            lb,
        });
    }, [
        { ...svcItem({ type: 'ClusterIP', clusterIP: '10.0.0.1', ports: [{ port: 80, targetPort: 80 }] }), metadata: { name: 'no-protocol' } },
        { ...svcItem({ clusterIP: '10.0.0.2', ports: [{ port: 80, protocol: 'TCP', targetPort: 80 }] }), metadata: { name: 'no-type' }, __noSort: true },
        { ...svcItem({ type: 'ClusterIP' }), metadata: { name: 'no-cluster-ip' }, __noSort: true },
        { ...svcItem({ type: 'ClusterIP', clusterIP: 'None', ports: [] }), metadata: { name: 'headless' } },
        { ...svcItem({ type: 'LoadBalancer', clusterIP: '10.0.0.3', ports: [{ port: 80, protocol: 'TCP', targetPort: 80, nodePort: 30001 }, { port: 443, protocol: 'TCP', targetPort: 443, nodePort: 0 }], lb: [{ ip: '1.1.1.1' }, { ip: '2.2.2.2' }], externalIPs: ['9.9.9.9', '8.8.8.8'] }), metadata: { name: 'two-lb-ips' } },
        { ...svcItem({ type: 'LoadBalancer', clusterIP: '10.0.0.4', ports: [{ port: 80, protocol: 'TCP', targetPort: 80 }], lb: [{ hostname: 'only-host.example.com' }] }), metadata: { name: 'lb-hostname-only' } },
        { ...svcItem({ type: 'LoadBalancer', clusterIP: '10.0.0.5', ports: [{ port: 80, protocol: 'TCP', targetPort: 80 }], lb: [{ hostname: 'h1.example.com' }, { ip: '3.3.3.3' }] }), metadata: { name: 'lb-host-then-ip' } },
        { ...svcItem({ type: 'LoadBalancer', clusterIP: '10.0.0.6', ports: [{ port: 80, protocol: 'TCP', targetPort: 80 }], lb: [{}] }), metadata: { name: 'lb-empty-entry' } },
        { ...svcItem({ type: 'ExternalName', externalName: 'db.example.com' }), metadata: { name: 'external-name' }, __noSort: true },
        { ...svcItem({ type: 'ExternalName', externalName: 'api.ext.io' }), metadata: { name: 'external-name-2' }, __noSort: true },
        { ...svcItem({ type: 'ExternalName' }), metadata: { name: 'external-name-empty' }, __noSort: true },
        { ...svcItem({ type: 'NodePort', clusterIP: '10.0.0.7', ports: [{ port: 1, protocol: 'TCP', targetPort: 'named' }, { port: 2, protocol: 'TCP', targetPort: 2, nodePort: 30002 }], selector: { b: '2', a: '1', '1': 'one' }, affinity: 'ClientIP' }), metadata: { name: 'multi-selector' } },
        annotatedEdge(svcItem({ type: 'ClusterIP', clusterIP: '10.0.0.8', ports: [{ port: 80, protocol: 'TCP', targetPort: 80 }] })),
        ...sameNameEdges(svcItem({ type: 'ClusterIP', clusterIP: '10.0.0.9', ports: [{ port: 80, protocol: 'TCP', targetPort: 80 }] })),
        noTimestampEdge(svcItem({ type: 'ClusterIP', clusterIP: '10.0.0.10', ports: [{ port: 80, protocol: 'TCP', targetPort: 80 }] })),
    ]);
}

// Ingresses as the API serves them: every path carries a backend object, the load balancer status
// is always present, and a service backend always has a port object.
function ingPath(path: string | undefined, svc = 'web', port: any = { number: 80 }) {
    const p: any = { backend: { service: { name: svc, port } }, pathType: 'Prefix' };
    if (path !== undefined) p.path = path;
    return p;
}

function ingRule(host: string | undefined, paths?: any[]) {
    const rule: any = {};
    if (host !== undefined) rule.host = host;
    if (paths) rule.http = { paths };
    return rule;
}

function ingItem(opts: { cls?: string; rules?: any[]; tls?: any[]; backend?: any; lb?: any[]; annotations?: Record<string, string> }) {
    const spec: any = {};
    if (opts.backend) spec.defaultBackend = opts.backend;
    if (opts.cls !== undefined) spec.ingressClassName = opts.cls;
    if (opts.rules) spec.rules = opts.rules;
    if (opts.tls) spec.tls = opts.tls;
    const item: any = { spec, status: { loadBalancer: opts.lb ? { ingress: opts.lb } : {} } };
    if (opts.annotations) item.__annotations = opts.annotations;
    return item;
}

const ingHosts = ['app.example.com', 'api.example.com', 'Zeta.example.com', '*.wild.example.com', 'café.example.com'];

export function ingressFixtures(): FixtureSet {
    const set = namespacedFixtures('ingresses', 163, (r, _name) => {
        const nrules = Math.floor(r() * 4);
        const rules = nrules > 0 ? Array.from({ length: nrules }, () => {
            const np = Math.floor(r() * 4);
            return ingRule(pick(r, [...ingHosts, undefined]), np > 0 ? Array.from({ length: np }, () => ingPath(pick(r, ['/', '/api', '/v1/users', undefined, '/static']))) : undefined);
        }) : undefined;
        return ingItem({
            cls: pick(r, [undefined, 'nginx', 'traefik', 'Alb']),
            rules,
            tls: pick(r, [undefined, [{ hosts: ['app.example.com'], secretName: 'tls-a' }], [{ secretName: 'a' }, { secretName: 'b' }]]),
            backend: pick(r, [undefined, undefined, { service: { name: 'fallback', port: { number: 8080 } } }, { service: { name: 'named-port', port: { name: 'http' } } }]),
            lb: pick(r, [undefined, [{ ip: '34.1.2.3' }], [{ hostname: 'elb.example.com' }], [{ ip: '34.1.2.3' }, { hostname: 'b.example.com' }], [{}]]),
            annotations: r() > 0.85 ? { 'kubernetes.io/ingress.class': pick(r, ['nginx', 'gce']) } : undefined,
        });
    }, [
        { ...ingItem({}), metadata: { name: 'empty' } },
        { ...ingItem({ rules: [ingRule(undefined, [ingPath('/')])] }), metadata: { name: 'host-less-rule' } },
        { ...ingItem({ rules: [ingRule('a.example.com'), ingRule('b.example.com')] }), metadata: { name: 'rules-without-http' } },
        { ...ingItem({ rules: [ingRule('a.example.com', [ingPath('/a'), ingPath('/b'), ingPath('/c'), ingPath('/d'), ingPath(undefined)])] }), metadata: { name: 'path-overflow' } },
        { ...ingItem({ rules: [ingRule('a.example.com', [ingPath('/a'), ingPath('/b'), ingPath('/c')])] }), metadata: { name: 'path-exact-three' } },
        { ...ingItem({ lb: [{ ip: '1.1.1.1' }], rules: [] }), metadata: { name: 'active-no-rules' } },
        { ...ingItem({ lb: [{}] }), metadata: { name: 'lb-empty-entry' } },
        { ...ingItem({ lb: [{}, { hostname: 'late.example.com' }], rules: [ingRule('x.example.com')] }), metadata: { name: 'lb-second-entry' } },
        { ...ingItem({ backend: { resource: { apiGroup: 'k8s.example.com', kind: 'StorageBucket', name: 'static' } } }), metadata: { name: 'resource-backend' } },
        { ...ingItem({ backend: { service: { name: 'svc', port: {} } } }), metadata: { name: 'portless-backend' } },
        { ...ingItem({ cls: 'nginx', annotations: { 'kubernetes.io/ingress.class': 'other' }, rules: [ingRule('c.example.com', [ingPath('/')])] }), metadata: { name: 'class-name-wins' } },
        { ...ingItem({ annotations: { 'kubernetes.io/ingress.class': 'legacy' }, rules: [ingRule('d.example.com', [ingPath('/')])] }), metadata: { name: 'class-annotation' } },
        { ...ingItem({ tls: [{ hosts: ['a'], secretName: 's' }, { hosts: ['b'], secretName: 't' }, { secretName: 'u' }], rules: [ingRule('e.example.com', [ingPath('/')])] }), metadata: { name: 'three-tls' } },
        annotatedEdge(ingItem({ rules: [ingRule('f.example.com', [ingPath('/')])] })),
        ...sameNameEdges(ingItem({ rules: [ingRule('g.example.com', [ingPath('/')])] })),
        noTimestampEdge(ingItem({ rules: [ingRule('h.example.com', [ingPath('/')])] })),
    ]);
    // Annotations ride beside the class field of the item builder; fold them into metadata.
    for (const item of set.items) {
        if (item.__annotations) {
            item.metadata.annotations = sortedGoOrder({ ...(item.metadata.annotations ?? {}), ...item.__annotations });
        }
        delete item.__annotations;
    }
    return set;
}

// ---------------------------------------------------------------------------------------------
// Workload types without pod-derived columns.

/** A pod template as the API serves it: metadata with a null creation timestamp, spec with containers. */
function podTemplate(containers: any[], extra: any = {}) {
    return { metadata: { creationTimestamp: null }, spec: { containers, ...extra } };
}

const containerSets: any[][] = [
    [{ name: 'app', image: 'nginx:1.25' }],
    [{ name: 'app', image: 'registry.example.com/team/app:v1' }, { name: 'sidecar', image: 'envoy:1.28' }],
    [{ name: 'c', image: 'quay.io/org/tool@sha256:0123abcd' }],
    [{ name: 'a', image: 'busybox' }, { name: 'b', image: 'alpine:3.19' }, { name: 'c', image: 'registry.example.com:5000/x/y:2' }],
];

const initContainerSets: Array<any[] | undefined> = [undefined, undefined, [{ name: 'init', image: 'busybox:1.36' }], [{ name: 'migrate', image: 'registry.example.com/team/migrate:v3' }]];

const matchLabelSets: Array<Record<string, string>> = [
    { app: 'web' },
    { app: 'web', tier: 'frontend' },
    { '10': 'ten', '9': 'nine', app: 'x' },
    { 'app.kubernetes.io/name': 'cache' },
];

const controllerOwners: Array<any[] | undefined> = [
    undefined,
    undefined,
    [{ apiVersion: 'apps/v1', kind: 'Deployment', name: 'web', uid: 'uid-owner-1', controller: true, blockOwnerDeletion: true }],
    [{ apiVersion: 'batch/v1', kind: 'CronJob', name: 'nightly', uid: 'uid-owner-2', controller: true, blockOwnerDeletion: true }],
    [{ apiVersion: 'argoproj.io/v1alpha1', kind: 'Rollout', name: 'canary', uid: 'uid-owner-3', controller: true }],
    [{ apiVersion: 'v1', kind: 'ConfigMap', name: 'not-controller', uid: 'uid-owner-4' }],
];

const ownerEdges = (extra: any) => [
    { ...extra, metadata: { name: 'owner-not-controller', ownerReferences: [{ apiVersion: 'v1', kind: 'ConfigMap', name: 'cm', uid: 'u-a', controller: false }] } },
    { ...extra, metadata: { name: 'owner-second-is-controller', ownerReferences: [{ apiVersion: 'v1', kind: 'ConfigMap', name: 'cm', uid: 'u-a' }, { apiVersion: 'apps/v1', kind: 'Deployment', name: 'web-2', uid: 'u-b', controller: true }] } },
    { ...extra, metadata: { name: 'owner-two-controllers', ownerReferences: [{ apiVersion: 'apps/v1', kind: 'StatefulSet', name: 'db', uid: 'u-c', controller: true }, { apiVersion: 'apps/v1', kind: 'Deployment', name: 'other', uid: 'u-d', controller: true }] } },
    { ...extra, metadata: { name: 'owner-unicode', ownerReferences: [{ apiVersion: 'example.com/v1', kind: 'Café', name: 'éclair', uid: 'u-e', controller: true }] } },
];

function randomPodSpecExtras(r: () => number) {
    const init = pick(r, initContainerSets);
    const sa = pick(r, [undefined, undefined, 'default', 'builder', 'Zeta-sa']);
    return { ...(init ? { initContainers: init } : {}), ...(sa ? { serviceAccountName: sa } : {}) };
}

function replicaSetItem(opts: {
    replicas?: number; status?: any; selector?: Record<string, string>; containers?: any[]; extraSpec?: any; owners?: any[];
}) {
    const item: any = {
        spec: {
            ...(opts.replicas !== undefined ? { replicas: opts.replicas } : {}),
            selector: { matchLabels: sortedGoOrder(opts.selector ?? { app: 'web' }) },
            template: podTemplate(opts.containers ?? containerSets[0], opts.extraSpec ?? {}),
        },
        status: { replicas: 0, ...(opts.status ?? {}) },
    };
    if (opts.owners) item.__owners = opts.owners;
    return item;
}

export function replicaSetFixtures(): FixtureSet {
    return namespacedFixtures('replicasets', 211, (r, _name) => {
        const replicas = pick(r, [0, 1, 2, 3, 5, 10]);
        const current = pick(r, [0, replicas, replicas, 1]);
        const ready = Math.min(current, pick(r, [0, current, current, 1]));
        return replicaSetItem({
            replicas: r() > 0.1 ? replicas : undefined,
            status: {
                replicas: current,
                ...(ready > 0 ? { readyReplicas: ready } : {}),
                ...(ready > 0 && r() > 0.5 ? { availableReplicas: ready } : {}),
                ...(r() > 0.8 ? { fullyLabeledReplicas: current } : {}),
            },
            selector: pick(r, matchLabelSets),
            containers: pick(r, containerSets),
            extraSpec: randomPodSpecExtras(r),
            owners: pick(r, controllerOwners),
        });
    }, [
        { ...replicaSetItem({}), metadata: { name: 'no-replicas' } },
        { ...replicaSetItem({ replicas: 3, status: { replicas: 3, readyReplicas: 2, availableReplicas: 2, fullyLabeledReplicas: 3 } }), metadata: { name: 'full-status' } },
        { ...replicaSetItem({ replicas: 0 }), metadata: { name: 'scaled-to-zero' } },
        { ...replicaSetItem({ replicas: 1, containers: [] }), metadata: { name: 'no-containers' } },
        { ...replicaSetItem({ replicas: 1, containers: [{ name: 'a', image: 'a:1' }, { name: 'b', image: 'b:2' }], extraSpec: { initContainers: [{ name: 'i1', image: 'i:1' }, { name: 'i2', image: 'j:2' }] } }), metadata: { name: 'init-containers' } },
        { ...replicaSetItem({ replicas: 1, extraSpec: { serviceAccount: 'legacy-sa' } }), metadata: { name: 'deprecated-service-account' } },
        { ...replicaSetItem({ replicas: 1, extraSpec: { serviceAccountName: 'new-sa', serviceAccount: 'old-sa' } }), metadata: { name: 'both-service-accounts' } },
        { ...replicaSetItem({ replicas: 1, selector: { b: '2', a: '1', '1': 'one' } }), metadata: { name: 'multi-selector' } },
        ...ownerEdges(replicaSetItem({ replicas: 1 })),
        annotatedEdge(replicaSetItem({ replicas: 1 })),
        ...sameNameEdges(replicaSetItem({ replicas: 1 })),
        noTimestampEdge(replicaSetItem({ replicas: 1 })),
    ]);
}

function jobItem(opts: {
    spec?: any; status?: any; containers?: any[]; extraSpec?: any; owners?: any[];
}) {
    const item: any = {
        spec: {
            ...(opts.spec ?? {}),
            selector: { matchLabels: { 'batch.kubernetes.io/controller-uid': 'x' } },
            template: podTemplate(opts.containers ?? containerSets[0], { restartPolicy: 'Never', ...(opts.extraSpec ?? {}) }),
        },
        status: opts.status ?? {},
    };
    if (opts.owners) item.__owners = opts.owners;
    return item;
}

const jobCond = (type: string, status = 'True') => ({ type, status, lastProbeTime: null, lastTransitionTime: null });

export function jobFixtures(): FixtureSet {
    return namespacedFixtures('jobs', 233, (r, _name) => {
        const startMin = pick(r, [undefined, 5, 90, 3000, 1500]);
        const durationMin = pick(r, [0, 1, 7, 61, 130]);
        const done = startMin !== undefined && r() > 0.4;
        const conditions = done ? pick(r, [[jobCond('Complete')], [jobCond('Failed')], [jobCond('Suspended', 'False'), jobCond('Complete')]]) : pick(r, [undefined, undefined, [jobCond('Suspended')]]);
        return jobItem({
            spec: {
                ...(pick(r, [true, false]) ? { parallelism: pick(r, [0, 1, 2, 5]) } : {}),
                ...(pick(r, [true, false]) ? { completions: pick(r, [1, 3, 10]) } : {}),
                ...(pick(r, [true, false]) ? { backoffLimit: pick(r, [0, 3, 6, 10]) } : {}),
                ...(r() > 0.8 ? { activeDeadlineSeconds: pick(r, [60, 3600]) } : {}),
            },
            status: {
                ...(startMin !== undefined ? { startTime: iso(startMin) } : {}),
                ...(done && startMin !== undefined ? { completionTime: iso(Math.max(startMin - durationMin, 0)) } : {}),
                ...(conditions ? { conditions } : {}),
                ...(r() > 0.5 ? { succeeded: pick(r, [1, 2, 3]) } : {}),
                ...(r() > 0.8 ? { failed: pick(r, [1, 4]) } : {}),
                ...(r() > 0.7 ? { active: pick(r, [1, 2]) } : {}),
            },
            containers: pick(r, containerSets),
            extraSpec: randomPodSpecExtras(r),
            owners: pick(r, controllerOwners),
        });
    }, [
        { ...jobItem({}), metadata: { name: 'bare' } },
        { ...jobItem({ spec: { parallelism: 0, completions: 0, backoffLimit: 0 } }), metadata: { name: 'explicit-zeros' } },
        { ...jobItem({ spec: { completions: 5, parallelism: 2, backoffLimit: 3, activeDeadlineSeconds: 120 }, status: { startTime: iso(30), completionTime: iso(20), succeeded: 5, conditions: [jobCond('Complete')] } }), metadata: { name: 'finished' } },
        { ...jobItem({ status: { startTime: iso(10), active: 1 } }), metadata: { name: 'running' } },
        { ...jobItem({ status: { startTime: iso(10), completionTime: iso(10) } }), metadata: { name: 'zero-duration' } },
        { ...jobItem({ status: { startTime: iso(3), completionTime: iso(0) } }), metadata: { name: 'three-minutes' } },
        { ...jobItem({ status: { startTime: iso(125), completionTime: iso(0) } }), metadata: { name: 'two-hours' } },
        { ...jobItem({ status: { conditions: [{ type: '', status: 'True' }] } }), metadata: { name: 'empty-condition-type' } },
        { ...jobItem({ status: { conditions: [jobCond('Failed'), jobCond('Complete')] } }), metadata: { name: 'two-conditions' } },
        { ...jobItem({ containers: [] }), metadata: { name: 'no-containers' } },
        { ...jobItem({ containers: [{ name: 'a', image: 'a:1' }, { name: 'b', image: 'b:2' }], extraSpec: { initContainers: [{ name: 'i1', image: 'i:1' }], serviceAccountName: 'job-sa', serviceAccount: 'old-sa' } }), metadata: { name: 'init-and-sa' } },
        { ...jobItem({ extraSpec: { serviceAccount: 'legacy-sa' } }), metadata: { name: 'deprecated-service-account' } },
        ...ownerEdges(jobItem({})),
        annotatedEdge(jobItem({})),
        ...sameNameEdges(jobItem({})),
        noTimestampEdge(jobItem({})),
    ]);
}

const nodeSelectorSets: Array<Record<string, string> | undefined> = [
    undefined, undefined,
    { 'kubernetes.io/os': 'linux' },
    { 'kubernetes.io/os': 'linux', disk: 'ssd' },
    { '10': 'ten', '9': 'nine', zone: 'a' },
];

function daemonSetItem(opts: { strategy?: string; selector?: Record<string, string>; containers?: any[]; extraSpec?: any; status?: any }) {
    return {
        spec: {
            selector: { matchLabels: sortedGoOrder(opts.selector ?? { app: 'web' }) },
            template: podTemplate(opts.containers ?? containerSets[0], opts.extraSpec ?? {}),
            ...(opts.strategy !== undefined ? { updateStrategy: opts.strategy ? { type: opts.strategy } : {} } : {}),
        },
        status: { currentNumberScheduled: 0, numberMisscheduled: 0, desiredNumberScheduled: 0, numberReady: 0, ...(opts.status ?? {}) },
    };
}

export function daemonSetFixtures(): FixtureSet {
    return namespacedFixtures('daemonsets', 257, (r, _name) => {
        const desired = pick(r, [0, 1, 3, 5, 12]);
        const ready = pick(r, [0, desired, desired, 1]);
        const ns = pick(r, nodeSelectorSets);
        return daemonSetItem({
            strategy: pick(r, [undefined, 'RollingUpdate', 'OnDelete']),
            selector: pick(r, matchLabelSets),
            containers: pick(r, containerSets),
            extraSpec: { ...randomPodSpecExtras(r), ...(ns ? { nodeSelector: sortedGoOrder(ns) } : {}) },
            status: {
                desiredNumberScheduled: desired,
                currentNumberScheduled: pick(r, [0, desired, desired]),
                numberMisscheduled: pick(r, [0, 0, 1]),
                numberReady: ready,
                ...(r() > 0.3 ? { updatedNumberScheduled: pick(r, [0, desired]) } : {}),
                ...(r() > 0.3 ? { numberAvailable: ready } : {}),
            },
        });
    }, [
        { ...daemonSetItem({}), metadata: { name: 'bare' } },
        { ...daemonSetItem({ strategy: '' }), metadata: { name: 'empty-strategy' } },
        { ...daemonSetItem({ strategy: 'OnDelete', status: { desiredNumberScheduled: 4, currentNumberScheduled: 4, numberReady: 3, numberAvailable: 3, updatedNumberScheduled: 4, numberMisscheduled: 1 } }), metadata: { name: 'full-status' } },
        { ...daemonSetItem({ containers: [] }), metadata: { name: 'no-containers' } },
        { ...daemonSetItem({ containers: [{ name: 'a', image: 'a:1' }, { name: 'b', image: 'b:2' }], extraSpec: { initContainers: [{ name: 'i1', image: 'i:1' }], serviceAccountName: 'ds-sa', serviceAccount: 'old-sa' } }), metadata: { name: 'init-and-sa' } },
        { ...daemonSetItem({ extraSpec: { serviceAccount: 'legacy-sa' } }), metadata: { name: 'deprecated-service-account' } },
        { ...daemonSetItem({ selector: { b: '2', a: '1', '1': 'one' }, extraSpec: { nodeSelector: sortedGoOrder({ z: '1', a: '2', '5': 'five' }) } }), metadata: { name: 'multi-selectors' } },
        annotatedEdge(daemonSetItem({})),
        ...sameNameEdges(daemonSetItem({})),
        noTimestampEdge(daemonSetItem({})),
    ]);
}

function deploymentItem(opts: {
    replicas?: number; strategy?: string; selector?: Record<string, string> | null; containers?: any[]; extraSpec?: any; status?: any; paused?: boolean;
}) {
    const spec: any = {
        ...(opts.replicas !== undefined ? { replicas: opts.replicas } : {}),
        selector: opts.selector === null ? { matchExpressions: [{ key: 'app', operator: 'Exists' }] } : { matchLabels: sortedGoOrder(opts.selector ?? { app: 'web' }) },
        template: podTemplate(opts.containers ?? containerSets[0], opts.extraSpec ?? {}),
        strategy: opts.strategy ? { type: opts.strategy } : {},
        ...(opts.paused ? { paused: true } : {}),
    };
    return { spec, status: opts.status ?? {} };
}

export function deploymentFixtures(): FixtureSet {
    return namespacedFixtures('deployments', 311, (r, _name) => {
        const replicas = pick(r, [undefined, 0, 1, 3, 10]);
        const total = pick(r, [0, 1, 3, 10]);
        const status: any = {};
        if (total > 0 || r() > 0.5) status.replicas = total;
        const ready = pick(r, [0, total, total, 1]);
        if (ready > 0) status.readyReplicas = Math.min(ready, Math.max(total, 1));
        const upd = pick(r, [0, total]);
        if (upd > 0) status.updatedReplicas = upd;
        const avail = pick(r, [0, ready]);
        if (avail > 0) status.availableReplicas = avail;
        return deploymentItem({
            replicas,
            strategy: pick(r, [undefined, 'RollingUpdate', 'Recreate']),
            selector: pick(r, matchLabelSets),
            containers: pick(r, containerSets),
            extraSpec: randomPodSpecExtras(r),
            status,
            paused: r() > 0.9,
        });
    }, [
        { ...deploymentItem({}), metadata: { name: 'bare' } },
        { ...deploymentItem({ replicas: 0, strategy: 'Recreate', status: { replicas: 2, readyReplicas: 1, updatedReplicas: 2, availableReplicas: 1 } }), metadata: { name: 'scaled-down-recreate' } },
        { ...deploymentItem({ replicas: 5, status: { replicas: 5, readyReplicas: 5, updatedReplicas: 5, availableReplicas: 5 }, paused: true }), metadata: { name: 'paused-full' } },
        { ...deploymentItem({ selector: null }), metadata: { name: 'expressions-only' } },
        { ...deploymentItem({ containers: [] }), metadata: { name: 'no-containers' } },
        { ...deploymentItem({ containers: [{ name: 'a', image: 'a:1' }, { name: 'b', image: 'b:2' }], extraSpec: { initContainers: [{ name: 'i1', image: 'i:1' }], serviceAccountName: 'dep-sa', serviceAccount: 'old-sa' } }), metadata: { name: 'init-and-sa' } },
        { ...deploymentItem({ extraSpec: { serviceAccount: 'legacy-sa' } }), metadata: { name: 'deprecated-service-account' } },
        { ...deploymentItem({ selector: { b: '2', a: '1', '1': 'one' } }), metadata: { name: 'multi-selectors' } },
        { ...deploymentItem({ selector: { app: 'web' } }), metadata: { name: 'twin-a', namespace: 'prod' } },
        { ...deploymentItem({ selector: { app: 'web' } }), metadata: { name: 'twin-b', namespace: 'prod' } },
        annotatedEdge(deploymentItem({})),
        ...sameNameEdges(deploymentItem({ selector: { app: 'same' } })),
        noTimestampEdge(deploymentItem({})),
    ]);
}

function statefulSetItem(opts: {
    replicas?: number; serviceName?: string; strategy?: string; policy?: string; claims?: number; selector?: Record<string, string> | null;
    containers?: any[]; extraSpec?: any; status?: any;
}) {
    const spec: any = {
        ...(opts.replicas !== undefined ? { replicas: opts.replicas } : {}),
        selector: opts.selector === null ? { matchExpressions: [{ key: 'app', operator: 'Exists' }] } : { matchLabels: sortedGoOrder(opts.selector ?? { app: 'web' }) },
        template: podTemplate(opts.containers ?? containerSets[0], opts.extraSpec ?? {}),
        ...(opts.claims ? { volumeClaimTemplates: Array.from({ length: opts.claims }, (_x, i) => ({ metadata: { name: `data-${i}`, creationTimestamp: null }, spec: { resources: {} }, status: {} })) } : {}),
        serviceName: opts.serviceName ?? '',
        ...(opts.policy ? { podManagementPolicy: opts.policy } : {}),
        updateStrategy: opts.strategy ? { type: opts.strategy } : {},
    };
    return { spec, status: { replicas: 0, ...(opts.status ?? {}) } };
}

export function statefulSetFixtures(): FixtureSet {
    return namespacedFixtures('statefulsets', 313, (r, _name) => {
        const total = pick(r, [0, 1, 3, 5]);
        const status: any = { replicas: total };
        const ready = pick(r, [0, total, total, 1]);
        if (ready > 0) status.readyReplicas = Math.min(ready, Math.max(total, 1));
        const cur = pick(r, [0, total]);
        if (cur > 0) status.currentReplicas = cur;
        const upd = pick(r, [0, total]);
        if (upd > 0) status.updatedReplicas = upd;
        return statefulSetItem({
            replicas: pick(r, [undefined, 0, 1, 3, 5]),
            serviceName: pick(r, ['', 'db-headless', 'Svc', 'cache']),
            strategy: pick(r, [undefined, 'RollingUpdate', 'OnDelete']),
            policy: pick(r, [undefined, 'OrderedReady', 'Parallel']),
            claims: pick(r, [0, 0, 1, 2]),
            selector: pick(r, matchLabelSets),
            containers: pick(r, containerSets),
            extraSpec: randomPodSpecExtras(r),
            status,
        });
    }, [
        { ...statefulSetItem({}), metadata: { name: 'bare' } },
        { ...statefulSetItem({ replicas: 3, serviceName: 'web', strategy: 'OnDelete', policy: 'Parallel', claims: 3, status: { replicas: 3, readyReplicas: 2, currentReplicas: 3, updatedReplicas: 1 } }), metadata: { name: 'full' } },
        { ...statefulSetItem({ replicas: 0 }), metadata: { name: 'scaled-to-zero' } },
        { ...statefulSetItem({ selector: null }), metadata: { name: 'expressions-only' } },
        { ...statefulSetItem({ containers: [] }), metadata: { name: 'no-containers' } },
        { ...statefulSetItem({ containers: [{ name: 'a', image: 'a:1' }, { name: 'b', image: 'b:2' }], extraSpec: { initContainers: [{ name: 'i1', image: 'i:1' }], serviceAccountName: 'sts-sa', serviceAccount: 'old-sa' } }), metadata: { name: 'init-and-sa' } },
        { ...statefulSetItem({ extraSpec: { serviceAccount: 'legacy-sa' } }), metadata: { name: 'deprecated-service-account' } },
        { ...statefulSetItem({ selector: { b: '2', a: '1', '1': 'one' } }), metadata: { name: 'multi-selectors' } },
        annotatedEdge(statefulSetItem({})),
        ...sameNameEdges(statefulSetItem({ selector: { app: 'same' } })),
        noTimestampEdge(statefulSetItem({})),
    ]);
}

/** A raw pod for the derived Pods column: only the fields the legacy status helpers and the selector match read. */
function derivedPod(uid: string, name: string, ns: string, labels: Record<string, string>, shape: PodShape): any {
    const metadata: any = { name, namespace: ns, uid, creationTimestamp: iso(500), ...(Object.keys(labels).length ? { labels: sortedGoOrder(labels) } : {}) };
    if (shape.deleting) metadata.deletionTimestamp = iso(1);
    const status: any = { ...(shape.phase !== undefined ? { phase: shape.phase } : {}), ...(shape.containers ? { containerStatuses: shape.containers.map((state, i) => ({ name: `c${i}`, state, ready: false, restartCount: 0, image: 'i', imageID: 'id' })) } : {}) };
    return { metadata, spec: { containers: [{ name: 'c0', image: 'i' }] }, status };
}

interface PodShape { phase?: string; containers?: any[]; deleting?: boolean }

const csRunning = { running: {} };
const csWaiting = (reason?: string) => ({ waiting: reason === undefined ? {} : { reason } });
const csDone = (exitCode: number) => ({ terminated: { exitCode } });

/** Pod shapes covering every branch of getEffectivePodStatus. */
const POD_SHAPES: PodShape[] = [
    { phase: 'Running', containers: [csRunning] },
    { phase: 'Pending' },
    { phase: 'Pending', containers: [csWaiting('ContainerCreating')] },
    { phase: 'Running', containers: [csWaiting('CrashLoopBackOff')] },
    { phase: 'Pending', containers: [csWaiting('ImagePullBackOff')] },
    { phase: 'Pending', containers: [csWaiting('ErrImagePull')] },
    { phase: 'Running', containers: [csRunning, csDone(0)] },
    { phase: 'Succeeded', containers: [csDone(0), csDone(0)] },
    { phase: 'Failed', containers: [csDone(1)] },
    { phase: 'Running', containers: [csRunning, csWaiting('CrashLoopBackOff')] },
    { phase: 'Running', containers: [csRunning, csDone(2)] },
    { phase: 'Pending', containers: [csWaiting()] },
    { phase: 'Pending', containers: [csWaiting('PodInitializing')] },
    { phase: 'Running', containers: [csWaiting('Weird'), csRunning] },
    { containers: [{}] },
    {},
    { phase: 'Running', containers: [csRunning], deleting: true },
];

export interface WorkloadPodState {
    name: string;
    /** The pod list the legacy usePods hook would hand the list, in list order (name, then uid). */
    pods: any[];
}

/**
 * Pod lists for the derived Pods column of a workload list: none, matching pods per workload (with
 * look-alikes that must not match), a churned list (statuses, labels and membership changed),
 * and the pods of one namespace only.
 */
export function workloadPodStates(workloads: any[]): WorkloadPodState[] {
    const build = (seed: number, churn: boolean): any[] => {
        const r = prng(seed);
        const pods: any[] = [];
        let n = 0;
        const add = (name: string, ns: string, labels: Record<string, string>, shape: PodShape) => {
            n += 1;
            pods.push(derivedPod(`uid-pod-${seed}-${String(n).padStart(4, '0')}`, name, ns, labels, shape));
        };
        workloads.forEach((w, wi) => {
            const sel = w.spec?.selector?.matchLabels;
            const ns = w.metadata.namespace ?? 'default';
            const count = churn ? pick(r, [0, 1, 2, 4]) : pick(r, [0, 1, 2, 3, 5]);
            for (let i = 0; i < count; i++) {
                add(`${w.metadata.name}-${wi}-${i}`, ns, { ...(sel ?? { app: 'orphan' }), ...(r() > 0.5 ? { 'pod-template-hash': `h${i}` } : {}) }, pick(r, POD_SHAPES));
            }
            if (sel && r() > 0.6) {
                // Look-alikes: wrong value, missing key, other namespace, extra label only.
                const [k] = Object.keys(sel);
                add(`${w.metadata.name}-wrongvalue`, ns, { ...sel, [k]: `${sel[k]}-x` }, pick(r, POD_SHAPES));
                add(`${w.metadata.name}-missingkey`, ns, { unrelated: 'y' }, pick(r, POD_SHAPES));
                add(`${w.metadata.name}-otherns`, `${ns}-other`, { ...sel }, pick(r, POD_SHAPES));
            }
        });
        // Unlabelled and empty-label pods never match anything.
        add('bare-pod', 'default', {}, { phase: 'Running', containers: [csRunning] });
        // Name, then uid, compared as UTF-8 bytes like the Go side.
        const enc = new TextEncoder();
        const bytes = (x: string, y: string) => {
            const a = enc.encode(x), b = enc.encode(y);
            for (let i = 0; i < Math.min(a.length, b.length); i++) if (a[i] !== b[i]) return a[i] - b[i];
            return a.length - b.length;
        };
        return pods.sort((a, b) => bytes(a.metadata.name, b.metadata.name) || bytes(a.metadata.uid, b.metadata.uid));
    };
    const first = build(701, false);
    const second = build(702, true);
    const prodOnly = first.filter(p => p.metadata.namespace === 'prod');
    return [
        { name: 'none', pods: [] },
        { name: 'pods', pods: first },
        { name: 'churned', pods: second },
        { name: 'one namespace', pods: prodOnly },
    ];
}

// ---------------------------------------------------------------------------------------------
// Persistent volume claims. Quantities are canonical (the Go backend prints them that way).

const pvcSizes = ['1Gi', '5Gi', '10Gi', '100Gi', '500Mi', '2Ti', '250Mi'];
const pvcModes: string[][] = [['ReadWriteOnce'], ['ReadWriteMany'], ['ReadOnlyMany', 'ReadWriteOnce'], ['ReadWriteOncePod'], ['ReadWriteOnce', 'ReadWriteMany', 'ReadOnlyMany']];

function pvcItem(opts: {
    phase?: string; volumeName?: string; capacity?: string; requests?: string; modes?: string[]; storageClass?: string; volumeMode?: string; selector?: Record<string, string>;
}) {
    const spec: any = {};
    if (opts.modes) spec.accessModes = opts.modes;
    if (opts.selector) spec.selector = { matchLabels: sortedGoOrder(opts.selector) };
    spec.resources = opts.requests !== undefined ? { requests: { storage: opts.requests } } : {};
    if (opts.volumeName !== undefined) spec.volumeName = opts.volumeName;
    if (opts.storageClass !== undefined) spec.storageClassName = opts.storageClass;
    if (opts.volumeMode !== undefined) spec.volumeMode = opts.volumeMode;
    const status: any = {};
    if (opts.phase !== undefined) status.phase = opts.phase;
    if (opts.capacity !== undefined) status.capacity = { storage: opts.capacity };
    return { spec, status };
}

export function pvcFixtures(): FixtureSet {
    return namespacedFixtures('persistentvolumeclaims', 281, (r, _name) => {
        const phase = pick(r, ['Bound', 'Bound', 'Pending', 'Lost']);
        const requested = pick(r, pvcSizes);
        const bound = phase === 'Bound';
        return pvcItem({
            phase,
            volumeName: bound ? `pvc-${Math.floor(r() * 1000)}` : undefined,
            capacity: bound ? pick(r, [requested, requested, pick(r, pvcSizes)]) : undefined,
            requests: requested,
            modes: pick(r, pvcModes),
            storageClass: pick(r, [undefined, 'standard', 'gp3', 'Fast-SSD']),
            volumeMode: pick(r, [undefined, 'Filesystem', 'Block']),
            selector: r() > 0.85 ? pick(r, matchLabelSets) : undefined,
        });
    }, [
        { ...pvcItem({ phase: 'Bound', requests: '1Gi', capacity: '1Gi', modes: ['ReadWriteOnce'], volumeName: 'pv-a' }), metadata: { name: 'plain' } },
        { ...pvcItem({ phase: 'Pending', requests: '5Gi', modes: ['ReadWriteOnce'] }), metadata: { name: 'pending-no-volume' } },
        { ...pvcItem({ phase: 'Pending', modes: [] }), metadata: { name: 'no-requests-no-modes' } },
        { ...pvcItem({ phase: 'Bound', requests: '1Gi', capacity: '2Gi', modes: ['ReadWriteMany'], volumeName: 'pv-b', storageClass: '' }), metadata: { name: 'empty-class-grown' } },
        { ...pvcItem({ phase: 'Bound', requests: '1Gi', capacity: '1Gi', modes: ['ReadWriteOnce'], volumeName: 'pv-c', volumeMode: '' }), metadata: { name: 'empty-volume-mode' } },
        { ...pvcItem({ phase: 'Bound', requests: '10Gi', capacity: '10Gi', modes: ['ReadOnlyMany'], volumeName: 'pv-d', selector: { b: '2', a: '1', '1': 'one' } }), metadata: { name: 'multi-selector' } },
        { ...pvcItem({ phase: 'Bound', requests: '1Gi', capacity: '1Gi', modes: ['Custom'], volumeName: 'pv-e' }), metadata: { name: 'unknown-access-mode' } },
        { ...pvcItem({ modes: ['ReadWriteOnce'], requests: '1Gi' }), metadata: { name: 'no-phase' }, __noSort: true },
        { ...pvcItem({ phase: 'Bound', requests: '3Gi', modes: ['ReadWriteOnce'], volumeName: 'pv-f' }), metadata: { name: 'bound-without-capacity' } },
        annotatedEdge(pvcItem({ phase: 'Bound', requests: '1Gi', capacity: '1Gi', modes: ['ReadWriteOnce'], volumeName: 'pv-g' })),
        ...sameNameEdges(pvcItem({ phase: 'Bound', requests: '1Gi', capacity: '1Gi', modes: ['ReadWriteOnce'], volumeName: 'pv-h' })),
        noTimestampEdge(pvcItem({ phase: 'Bound', requests: '1Gi', capacity: '1Gi', modes: ['ReadWriteOnce'], volumeName: 'pv-i' })),
    ]);
}

// ---------------------------------------------------------------------------------------------
// Pods and nodes. Shapes follow what the Go backend serialises for core/v1 Pod and
// Node: container statuses always carry name, state ({} when unset), lastState, ready,
// restartCount, image and imageID; node status always carries nodeInfo.

const podNodeNames = ['node-a', 'node-b', 'ip-10-0-0-1.ec2.internal', 'Node-Z'];

const waiting = (reason?: string, message?: string) => ({ waiting: { ...(reason ? { reason } : {}), ...(message ? { message } : {}) } });
const running = () => ({ running: { startedAt: null } });
const terminated = (exitCode: number, reason?: string) => ({ terminated: { exitCode, ...(reason ? { reason } : {}), startedAt: null, finishedAt: null } });

function containerStatus(name: string, state: any, restartCount = 0, image = 'img:1'): any {
    return { name, state, lastState: {}, ready: !!state.running, restartCount, image, imageID: '' };
}

interface PodOpts {
    phase?: string;
    containers?: any[];
    initContainers?: any[];
    statuses?: any[];
    initStatuses?: any[];
    nodeName?: string;
    podIP?: string;
    hostIP?: string;
    qos?: string;
    serviceAccountName?: string;
    serviceAccount?: string;
    priorityClassName?: string;
    owners?: any[];
    deletion?: string;
}

function podItem(o: PodOpts): any {
    const containers = o.containers ?? containerSets[0];
    const spec: any = {
        containers: containers.map(c => ({ ...c, resources: {} })),
        ...(o.initContainers ? { initContainers: o.initContainers.map(c => ({ ...c, resources: {} })) } : {}),
        ...(o.nodeName ? { nodeName: o.nodeName } : {}),
        ...(o.serviceAccountName ? { serviceAccountName: o.serviceAccountName } : {}),
        ...(o.serviceAccount ? { serviceAccount: o.serviceAccount } : {}),
        ...(o.priorityClassName ? { priorityClassName: o.priorityClassName } : {}),
    };
    const status: any = {
        ...(o.phase ? { phase: o.phase } : {}),
        ...(o.hostIP ? { hostIP: o.hostIP } : {}),
        ...(o.podIP ? { podIP: o.podIP } : {}),
        ...(o.initStatuses ? { initContainerStatuses: o.initStatuses } : {}),
        ...(o.statuses ? { containerStatuses: o.statuses } : {}),
        ...(o.qos ? { qosClass: o.qos } : {}),
    };
    const item: any = { spec, status };
    if (o.owners) item.__owners = o.owners;
    if (o.deletion) item.__meta = { deletionTimestamp: o.deletion };
    return item;
}

/** A random pod: one of the status profiles the list has to tell apart. */
function randomPod(r: () => number): any {
    const containers = pick(r, containerSets);
    const profile = pick(r, ['running', 'running', 'running', 'pending', 'creating', 'crash', 'pullback', 'errpull', 'succeeded', 'failed', 'initerr', 'initcrash', 'initrun', 'initdone', 'terminating', 'nophase', 'mixed']);
    const restarts = pick(r, [0, 0, 0, 1, 2, 5, 12]);
    const scheduled = profile !== 'pending';
    const base: PodOpts = {
        containers,
        nodeName: scheduled ? pick(r, podNodeNames) : undefined,
        podIP: scheduled && r() > 0.2 ? `10.1.${Math.floor(r() * 4)}.${Math.floor(r() * 250)}` : undefined,
        hostIP: scheduled && r() > 0.2 ? `192.168.0.${Math.floor(r() * 250)}` : undefined,
        qos: pick(r, [undefined, 'Guaranteed', 'Burstable', 'BestEffort']),
        serviceAccountName: pick(r, [undefined, undefined, 'default', 'builder', 'Zeta-sa']),
        priorityClassName: pick(r, [undefined, undefined, 'system-cluster-critical', 'low']),
        owners: pick(r, controllerOwners),
    };
    const statusesFor = (state: any, n = restarts) => containers.map((c, i) => containerStatus(c.name, state, i === 0 ? n : 0, c.image));
    switch (profile) {
        case 'pending': return podItem({ ...base, phase: 'Pending' });
        case 'creating': return podItem({ ...base, phase: 'Pending', statuses: statusesFor(waiting('ContainerCreating'), 0) });
        case 'crash': return podItem({ ...base, phase: 'Running', statuses: statusesFor(waiting('CrashLoopBackOff'), Math.max(restarts, 3)) });
        case 'pullback': return podItem({ ...base, phase: 'Pending', statuses: statusesFor(waiting('ImagePullBackOff'), 0) });
        case 'errpull': return podItem({ ...base, phase: 'Pending', statuses: statusesFor(waiting('ErrImagePull'), 0) });
        case 'succeeded': return podItem({ ...base, phase: 'Succeeded', statuses: statusesFor(terminated(0, 'Completed')) });
        case 'failed': return podItem({ ...base, phase: 'Failed', statuses: statusesFor(terminated(1, 'Error')) });
        case 'initerr': return podItem({ ...base, phase: 'Pending', initContainers: initContainerSets[2], initStatuses: [containerStatus('init', terminated(2, 'Error'))], statuses: statusesFor(waiting('PodInitializing'), 0) });
        case 'initcrash': return podItem({ ...base, phase: 'Pending', initContainers: initContainerSets[2], initStatuses: [containerStatus('init', waiting('CrashLoopBackOff'), 4)], statuses: statusesFor(waiting('PodInitializing'), 0) });
        case 'initrun': return podItem({ ...base, phase: 'Pending', initContainers: initContainerSets[3], initStatuses: [containerStatus('migrate', running())], statuses: statusesFor(waiting('PodInitializing'), 0) });
        case 'initdone': return podItem({ ...base, phase: 'Running', initContainers: initContainerSets[2], initStatuses: [containerStatus('init', terminated(0, 'Completed'))], statuses: statusesFor(running()) });
        case 'terminating': return podItem({ ...base, phase: 'Running', statuses: statusesFor(running()), deletion: iso(2) });
        case 'nophase': return podItem({ ...base, statuses: statusesFor(running()) });
        case 'mixed': return podItem({ ...base, phase: 'Running', statuses: containers.map((c, i) => containerStatus(c.name, [running(), waiting('CrashLoopBackOff'), terminated(0), terminated(3)][i % 4], i + restarts, c.image)) });
        default: return podItem({ ...base, phase: 'Running', statuses: statusesFor(running()) });
    }
}

export function podFixtures(): FixtureSet {
    const plain = (extra: PodOpts = {}) => podItem({ phase: 'Running', nodeName: 'node-a', podIP: '10.1.0.5', hostIP: '192.168.0.9', statuses: [containerStatus('app', running(), 1, 'nginx:1.25')], ...extra });
    return namespacedFixtures('pods', 311, (r, _name) => randomPod(r), [
        { ...plain(), metadata: { name: 'plain' } },
        { ...podItem({}), metadata: { name: 'empty-status' }, __noSort: true },
        { ...podItem({ phase: 'Running', containers: [] }), metadata: { name: 'no-containers' } },
        { ...plain({ containers: [{ name: 'a', image: 'a:1' }, { name: 'b', image: 'b:2' }], statuses: [containerStatus('a', running(), 2), containerStatus('b', running(), 3)] }), metadata: { name: 'restarts-sum' } },
        { ...plain({ serviceAccount: 'legacy-sa' }), metadata: { name: 'deprecated-service-account' } },
        { ...plain({ serviceAccountName: 'new-sa', serviceAccount: 'old-sa' }), metadata: { name: 'both-service-accounts' } },
        { ...plain({ initContainers: [{ name: 'i1', image: 'i:1' }, { name: 'i2', image: 'j:2' }] }), metadata: { name: 'init-containers' } },
        { ...podItem({ phase: 'Running', statuses: [containerStatus('a', running()), containerStatus('b', terminated(0, 'Completed')), containerStatus('c', waiting('CrashLoopBackOff'), 7)], containers: containerSets[3] }), metadata: { name: 'three-states' } },
        { ...podItem({ phase: 'Running', statuses: [{ name: 'app', state: {}, lastState: {}, ready: false, restartCount: 0, image: 'x', imageID: '' }] }), metadata: { name: 'state-unset' } },
        { ...podItem({ phase: 'Pending', statuses: [containerStatus('app', waiting())] }), metadata: { name: 'waiting-no-reason' } },
        { ...podItem({ phase: 'Running', initContainers: initContainerSets[2], initStatuses: [containerStatus('init', terminated(0)), containerStatus('init2', terminated(1))] }), metadata: { name: 'second-init-failed' } },
        { ...podItem({ phase: 'Running', initContainers: initContainerSets[2], initStatuses: [containerStatus('init', running())] }), metadata: { name: 'init-running-state' } },
        { ...podItem({ phase: 'Running', initContainers: initContainerSets[2], initStatuses: [containerStatus('init', {})] }), metadata: { name: 'init-state-unset' } },
        { ...plain({ deletion: iso(1) }), metadata: { name: 'terminating-edge' } },
        { ...plain({ phase: 'Weird' }), metadata: { name: 'weird-phase' } },
        { ...plain({ qos: 'Guaranteed', priorityClassName: 'system-node-critical' }), metadata: { name: 'qos-priority' } },
        { ...plain(), metadata: { name: 'labelled-pod', labels: { '10': 'ten', app: 'web', tier: 'frontend' } } },
        ...ownerEdges(plain()),
        annotatedEdge(plain()),
        ...sameNameEdges(plain()),
        noTimestampEdge(plain()),
    ]);
}

// Node items. Roles ride as labels; the list reads conditions, addresses, taints and nodeInfo.
const nodeCondition = (type: string, status: string) => ({ type, status, lastHeartbeatTime: null, lastTransitionTime: null });

interface NodeOpts {
    ready?: string;
    pressure?: string[];
    network?: boolean;
    unschedulable?: boolean;
    taints?: any[];
    addresses?: any[];
    version?: string;
    info?: any;
    roles?: string[];
    podCIDR?: string;
    noReadyCondition?: boolean;
}

function nodeItem(o: NodeOpts): any {
    const conditions: any[] = [];
    for (const t of ['MemoryPressure', 'DiskPressure', 'PIDPressure']) conditions.push(nodeCondition(t, o.pressure?.includes(t) ? 'True' : 'False'));
    if (!o.noReadyCondition) conditions.push(nodeCondition('Ready', o.ready ?? 'True'));
    if (o.network !== undefined) conditions.push(nodeCondition('NetworkUnavailable', o.network ? 'True' : 'False'));
    const nodeInfo = {
        machineID: '', systemUUID: '', bootID: '', kernelVersion: '5.15.0-1', osImage: 'Ubuntu 22.04', containerRuntimeVersion: 'containerd://1.7.2',
        kubeletVersion: o.version ?? 'v1.29.3', kubeProxyVersion: 'v1.29.3', operatingSystem: 'linux', architecture: 'amd64',
        ...(o.info ?? {}),
    };
    const item: any = {
        spec: {
            ...(o.podCIDR ? { podCIDR: o.podCIDR } : {}),
            ...(o.unschedulable ? { unschedulable: true } : {}),
            ...(o.taints ? { taints: o.taints } : {}),
        },
        status: { conditions, addresses: o.addresses ?? [{ type: 'InternalIP', address: '10.0.0.5' }, { type: 'Hostname', address: 'h' }], nodeInfo },
    };
    if (o.roles) item.__labels = Object.fromEntries(o.roles.map(role => [`node-role.kubernetes.io/${role}`, '']));
    return item;
}

const nodeTaints = [
    undefined,
    [{ key: 'dedicated', value: 'gpu', effect: 'NoSchedule' }],
    [{ key: 'node.kubernetes.io/unreachable', effect: 'NoExecute' }, { key: 'a', value: 'b', effect: 'PreferNoSchedule' }],
];

export function nodeFixtures(): FixtureSet {
    return clusterFixtures('nodes', 331, (r, _name) => nodeItem({
        ready: pick(r, ['True', 'True', 'True', 'False', 'Unknown']),
        pressure: pick(r, [[], [], [], ['MemoryPressure'], ['DiskPressure', 'PIDPressure']]),
        network: pick(r, [undefined, undefined, false, true]),
        unschedulable: r() > 0.8,
        taints: pick(r, nodeTaints),
        version: pick(r, ['v1.29.3', 'v1.28.9', 'v1.30.0-rc.1', 'v1.9.0', 'v1.10.1']),
        roles: pick(r, [undefined, ['control-plane'], ['worker'], ['control-plane', 'master'], ['10', 'etcd']]),
        addresses: pick(r, [undefined, [{ type: 'InternalIP', address: '10.0.0.7' }, { type: 'ExternalIP', address: '34.1.2.3' }], [{ type: 'Hostname', address: 'x' }], [{ type: 'ExternalIP', address: '35.0.0.1' }, { type: 'InternalIP', address: '10.0.1.1' }]]),
        info: pick(r, [undefined, { osImage: 'Flatcar Container Linux', kernelVersion: '6.1.0', architecture: 'arm64' }, { osImage: 'Bottlerocket', containerRuntimeVersion: 'cri-o://1.28' }]),
        podCIDR: pick(r, [undefined, '10.244.1.0/24']),
    }), [
        { ...nodeItem({}), metadata: { name: 'plain' } },
        { ...nodeItem({ noReadyCondition: true }), metadata: { name: 'no-ready-condition' } },
        { ...nodeItem({ ready: 'False', pressure: ['MemoryPressure', 'DiskPressure', 'PIDPressure'], network: true, unschedulable: true }), metadata: { name: 'everything-wrong' } },
        { ...nodeItem({ unschedulable: true }), metadata: { name: 'cordoned' } },
        { ...nodeItem({ taints: [{ key: 'k', effect: 'NoSchedule' }, { key: 'k2', value: 'v', effect: 'NoExecute', timeAdded: null }] }), metadata: { name: 'taints-empty-value' } },
        { ...nodeItem({ roles: ['control-plane', 'etcd', 'zeta'] }), metadata: { name: 'multi-role', labels: { 'node-role.kubernetes.io/control-plane': '', 'node-role.kubernetes.io/etcd': '', 'node-role.kubernetes.io/zeta': '', 'kubernetes.io/os': 'linux' } } },
        { ...nodeItem({ addresses: [] }), metadata: { name: 'no-addresses' } },
        { ...nodeItem({ addresses: [{ type: 'InternalIP', address: '10.0.0.1' }, { type: 'InternalIP', address: '10.0.0.2' }] }), metadata: { name: 'two-internal' } },
        { ...nodeItem({ version: '' }), metadata: { name: 'empty-version' } },
        { ...nodeItem({}), metadata: { name: 'annotated-node', annotations: { note: 'Hello World', 'a/b': 'c=d' }, labels: { '10': 'ten', tier: 'x' } } },
        { ...nodeItem({}), metadata: { name: 'no-timestamp', creationTimestamp: null } },
    ], ['uid-edge-11']);
}

// ---------------------------------------------------------------------------------------------
// Metrics overlay states. Each state is a raw backend result (what GetPodMetrics and friends
// return); the golden generator runs the legacy transform over it, so the Go transform and the
// view overlay are proven against the code the legacy list really uses.

export interface OverlayRaw {
    name: string;
    /** metricsAvailable as the list sees it: null before the first answer. */
    available: boolean | null;
    /** The raw result handed to the legacy transform; null leaves the legacy map empty. */
    raw: any;
    /** The hook keeps its last map while reporting unavailable (a poll that threw). */
    stale?: boolean;
}

const GI = 1024 * 1024 * 1024;

function rawPodMetric(r: () => number, ns: string, name: string, nodeName: string, shape: 'plain' | 'tie' | 'half'): any {
    const nodeCpuCapacity = pick(r, [0, 2000, 4000, 8000]);
    const nodeMemCapacity = pick(r, [0, 8 * GI, 16 * GI, 64 * GI]);
    let cpuUsage = Math.floor(r() * 900);
    let memoryUsage = Math.floor(r() * 4 * GI);
    if (shape === 'tie') { cpuUsage = nodeCpuCapacity * 0.05; memoryUsage = nodeMemCapacity * 0.1; }
    // Exact .5 percentages: Math.round rounds them up.
    if (shape === 'half') { cpuUsage = nodeCpuCapacity / 8; memoryUsage = nodeMemCapacity / 16; }
    cpuUsage = Math.floor(cpuUsage);
    memoryUsage = Math.floor(memoryUsage);
    const cpuRequested = pick(r, [0, 100, 250, 500, 1000]);
    const memRequested = pick(r, [0, GI / 4, GI, 2 * GI]);
    return {
        namespace: ns, name, nodeName, cpuUsage, memoryUsage, cpuRequested, memRequested,
        cpuCommitted: Math.max(cpuUsage, cpuRequested), memCommitted: Math.max(memoryUsage, memRequested),
        nodeCpuCapacity, nodeMemCapacity,
    };
}

/** Pod metric states over the given pod fixtures: none, two ticks (the second reorders), unavailable, error with stale data. */
export function podOverlayStates(pods: any[]): OverlayRaw[] {
    const build = (seed: number, shapeOf: (i: number) => 'plain' | 'tie' | 'half', skip: (i: number) => boolean) => {
        const r = prng(seed);
        return {
            available: true,
            metrics: pods.filter((_p, i) => !skip(i)).map((p, i) => rawPodMetric(r, p.metadata.namespace, p.metadata.name, p.spec?.nodeName ?? 'node-a', shapeOf(i))),
        };
    };
    return [
        { name: 'none', available: null, raw: null },
        { name: 'tick1', available: true, raw: build(901, i => (i % 5 === 0 ? 'tie' : i % 7 === 0 ? 'half' : 'plain'), i => i % 4 === 3) },
        { name: 'tick2', available: true, raw: build(902, i => (i % 3 === 0 ? 'tie' : i % 11 === 0 ? 'half' : 'plain'), i => i % 6 === 5) },
        { name: 'unavailable', available: false, raw: { available: false, metrics: [] } },
        // The legacy hook keeps its last map when a poll throws, while available turns false.
        { name: 'stale', available: false, stale: true, raw: build(901, i => (i % 5 === 0 ? 'tie' : i % 7 === 0 ? 'half' : 'plain'), i => i % 4 === 3) },
    ];
}

function rawNodeMetric(r: () => number, name: string, shape: 'plain' | 'tie' | 'half'): any {
    const cpuCapacity = pick(r, [0, 2000, 4000, 16000]);
    const memCapacity = pick(r, [0, 8 * GI, 32 * GI]);
    let cpuUsage = Math.floor(r() * 3000);
    let memoryUsage = Math.floor(r() * 16 * GI);
    if (shape === 'tie') { cpuUsage = cpuCapacity * 0.25; memoryUsage = memCapacity * 0.5; }
    if (shape === 'half') { cpuUsage = cpuCapacity / 8; memoryUsage = memCapacity / 16; }
    cpuUsage = Math.floor(cpuUsage);
    memoryUsage = Math.floor(memoryUsage);
    const cpuRequested = pick(r, [0, 500, 2000, 6000]);
    const memRequested = pick(r, [0, GI, 8 * GI]);
    const podCapacity = pick(r, [0, 110, 110, 30]);
    return {
        name, cpuUsage, memoryUsage, cpuCapacity, memCapacity, cpuRequested, memRequested,
        cpuCommitted: cpuRequested + cpuUsage, memCommitted: memRequested + memoryUsage,
        podCount: Math.min(podCapacity, Math.floor(r() * 120)), podCapacity,
    };
}

export function nodeOverlayStates(nodes: any[]): OverlayRaw[] {
    const build = (seed: number, shapeOf: (i: number) => 'plain' | 'tie' | 'half', skip: (i: number) => boolean) => {
        const r = prng(seed);
        return { available: true, metrics: nodes.filter((_n, i) => !skip(i)).map((n, i) => rawNodeMetric(r, n.metadata.name, shapeOf(i))) };
    };
    return [
        { name: 'none', available: null, raw: null },
        { name: 'tick1', available: true, raw: build(911, i => (i % 4 === 0 ? 'tie' : i % 6 === 0 ? 'half' : 'plain'), i => i % 5 === 4) },
        { name: 'tick2', available: true, raw: build(912, i => (i % 3 === 0 ? 'tie' : 'plain'), i => i % 7 === 6) },
        { name: 'unavailable', available: false, raw: { available: false, metrics: [] } },
        { name: 'stale', available: false, stale: true, raw: build(911, i => (i % 4 === 0 ? 'tie' : i % 6 === 0 ? 'half' : 'plain'), i => i % 5 === 4) },
    ];
}

/** Namespace metrics aggregate both pod and node results; a state carries both raw results. */
export interface NamespaceOverlayRaw {
    name: string;
    available: boolean | null;
    raw: { pods: any; nodes: any } | null;
}

export function namespaceOverlayStates(namespaces: any[]): NamespaceOverlayRaw[] {
    const nsNames = ['default', 'kube-system', 'kube-public', 'alpha', 'zeta', 'x', 'ghost-ns'];
    const build = (seed: number, perNs: number) => {
        const r = prng(seed);
        const pods: any[] = [];
        nsNames.forEach((ns, ni) => {
            const n = ni % 2 === 0 ? perNs : Math.max(1, perNs - 1);
            for (let k = 0; k < n; k++) pods.push(rawPodMetric(r, ns, `p-${ni}-${k}`, 'node-a', pick(r, ['plain', 'tie', 'half'])));
        });
        const nodes = Array.from({ length: 3 }, (_v, i) => rawNodeMetric(r, `n${i}`, 'plain')).map(n => ({ ...n, cpuCapacity: n.cpuCapacity || 4000, memCapacity: n.memCapacity || 8 * GI }));
        return { pods: { available: true, metrics: pods }, nodes: { available: true, metrics: nodes } };
    };
    void namespaces;
    const first = build(921, 4);
    return [
        { name: 'none', available: null, raw: null },
        { name: 'tick1', available: true, raw: first },
        { name: 'tick2', available: true, raw: build(922, 6) },
        { name: 'unavailable', available: false, raw: null },
        { name: 'zero-capacity', available: true, raw: { pods: first.pods, nodes: { available: true, metrics: [] } } },
    ];
}
