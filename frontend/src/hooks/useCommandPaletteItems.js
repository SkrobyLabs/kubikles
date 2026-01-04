import { useState, useEffect, useMemo } from 'react';
import { ListCRDs } from '../../wailsjs/go/main/App';
import { useK8s } from '../context/K8sContext';

// Static menu structure matching Sidebar.jsx
const menuGroups = [
    {
        title: 'Cluster',
        items: [
            { id: 'nodes', label: 'Nodes' },
            { id: 'namespaces', label: 'Namespaces' },
            { id: 'events', label: 'Events' },
            { id: 'priorityclasses', label: 'Priority Classes' },
        ]
    },
    {
        title: 'Workloads',
        items: [
            { id: 'pods', label: 'Pods' },
            { id: 'deployments', label: 'Deployments' },
            { id: 'statefulsets', label: 'StatefulSets' },
            { id: 'daemonsets', label: 'DaemonSets' },
            { id: 'replicasets', label: 'ReplicaSets' },
            { id: 'jobs', label: 'Jobs' },
            { id: 'cronjobs', label: 'CronJobs' },
        ]
    },
    {
        title: 'Config',
        items: [
            { id: 'configmaps', label: 'ConfigMaps' },
            { id: 'secrets', label: 'Secrets' },
            { id: 'hpas', label: 'HPAs' },
            { id: 'pdbs', label: 'PDBs' },
            { id: 'resourcequotas', label: 'Resource Quotas' },
            { id: 'limitranges', label: 'Limit Ranges' },
            { id: 'leases', label: 'Leases' },
        ]
    },
    {
        title: 'Network',
        items: [
            { id: 'services', label: 'Services' },
            { id: 'endpoints', label: 'Endpoints' },
            { id: 'endpointslices', label: 'Endpoint Slices' },
            { id: 'ingresses', label: 'Ingresses' },
            { id: 'ingressclasses', label: 'Ingress Classes' },
            { id: 'networkpolicies', label: 'Network Policies' },
            { id: 'portforwards', label: 'Port Forwards' },
        ]
    },
    {
        title: 'Storage',
        items: [
            { id: 'pvcs', label: 'PVCs' },
            { id: 'pvs', label: 'PVs' },
            { id: 'storageclasses', label: 'Storage Classes' },
            { id: 'csidrivers', label: 'CSI Drivers' },
            { id: 'csinodes', label: 'CSI Nodes' },
        ]
    },
    {
        title: 'Helm',
        items: [
            { id: 'helmreleases', label: 'Releases' },
            { id: 'helmrepos', label: 'Chart Sources' },
        ]
    },
    {
        title: 'Access Control',
        items: [
            { id: 'serviceaccounts', label: 'Service Accounts' },
            { id: 'roles', label: 'Roles' },
            { id: 'clusterroles', label: 'Cluster Roles' },
            { id: 'rolebindings', label: 'Role Bindings' },
            { id: 'clusterrolebindings', label: 'Cluster Role Bindings' },
        ]
    },
    {
        title: 'Admission Control',
        items: [
            { id: 'validatingwebhooks', label: 'Validating Webhooks' },
            { id: 'mutatingwebhooks', label: 'Mutating Webhooks' },
        ]
    }
];

export const useCommandPaletteItems = () => {
    const { currentContext } = useK8s();
    const [crds, setCRDs] = useState([]);
    const [loading, setLoading] = useState(false);

    // Fetch CRDs when context is available
    useEffect(() => {
        if (!currentContext) return;

        const fetchCRDs = async () => {
            setLoading(true);
            try {
                const list = await ListCRDs();
                setCRDs(list || []);
            } catch (err) {
                console.error('Failed to fetch CRDs for command palette:', err);
                setCRDs([]);
            } finally {
                setLoading(false);
            }
        };

        fetchCRDs();
    }, [currentContext]);

    // Build flat list of all navigable items
    const items = useMemo(() => {
        const result = [];

        // Add static menu items
        for (const group of menuGroups) {
            for (const item of group.items) {
                result.push({
                    id: item.id,
                    label: item.label,
                    path: `${group.title} > ${item.label}`,
                    viewId: item.id,
                    type: 'builtin'
                });
            }
        }

        // Add CRD definitions link
        result.push({
            id: 'crds',
            label: 'Definitions',
            path: 'Custom Resources > Definitions',
            viewId: 'crds',
            type: 'builtin'
        });

        // Add CRD instances grouped by API group
        for (const crd of crds) {
            const group = crd.spec?.group || 'unknown';
            const kind = crd.spec?.names?.kind;
            const plural = crd.spec?.names?.plural;
            const versions = crd.spec?.versions || [];
            const storageVersion = versions.find(v => v.storage)?.name || versions[0]?.name || 'v1';
            const namespaced = crd.spec?.scope === 'Namespaced';

            if (kind && plural) {
                const viewId = `cr:${group}:${storageVersion}:${plural}:${kind}:${namespaced}`;
                result.push({
                    id: `cr-${group}-${kind}`,
                    label: kind,
                    path: `Custom Resources > ${group} > ${kind}`,
                    viewId: viewId,
                    type: 'crd',
                    group: group
                });
            }
        }

        return result;
    }, [crds]);

    return { items, loading };
};
