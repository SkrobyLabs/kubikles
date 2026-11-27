// Which App methods may travel over the bulk GET route. This mirrors
// pkg/bulk/allowlist.go; bulkMethods.parity.test.ts parses that file and fails
// when the two drift.

export const LIST_PREFIX = 'List';
export const YAML_SUFFIXES = ['Yaml', 'YAML'];
export const HISTORY_SUFFIXES = ['History', 'HistoryRange'];
export const EXTRA_BULK_METHODS = [
    'GetPodMetrics',
    'GetNodeMetrics',
    'GetPodLogsBefore',
    'GetPodLogsAfter',
    'GetAllContainersLogsBefore',
    'GetAllContainersLogsAfter',
    'GetAllPodsLogsBefore',
    'GetAllPodsLogsAfter',
];
// Methods that match a naming rule but are not read-only (ListPodFiles execs in
// the pod and may add an ephemeral container).
export const DENIED_METHODS = ['ListPodFiles'];
export const FORBIDDEN_PREFIXES = [
    'Create', 'Delete', 'Update', 'Apply', 'Restart', 'Scale', 'Evict', 'Exec',
    'Set', 'Save', 'Import', 'Install', 'Uninstall', 'Start', 'Stop', 'Cancel',
];

export function isBulkMethod(name: string): boolean {
    if (!name) return false;
    if (DENIED_METHODS.includes(name)) return false;
    for (const p of FORBIDDEN_PREFIXES) if (name.startsWith(p)) return false;
    if (name.startsWith(LIST_PREFIX)) return true;
    if (name.startsWith('Get')) {
        for (const s of YAML_SUFFIXES) if (name.endsWith(s)) return true;
        for (const s of HISTORY_SUFFIXES) if (name.endsWith(s)) return true;
    }
    return EXTRA_BULK_METHODS.includes(name);
}
