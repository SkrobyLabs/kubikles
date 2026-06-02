import type { K8sPod, K8sContainerStatus } from '~/types/k8s';

export interface ResolvedLogTarget {
    namespace: string;
    pod: string;
    containers: string[];
    siblingPods: string[];
    podContainerMap: Record<string, string[]>;
    ownerName?: string;
    podCreationTime?: string;
}

const hasRunningState = (status?: K8sContainerStatus): boolean => Boolean(status?.state?.running);

const statusByName = (statuses: K8sContainerStatus[] = []): Map<string, K8sContainerStatus> =>
    new Map(statuses.map((status) => [status.name, status]));

const unique = (values: string[]): string[] => Array.from(new Set(values.filter(Boolean)));

export const getLogContainerNames = (pod: K8sPod): string[] => {
    const initNames = (pod.spec?.initContainers || []).map((container) => container.name);
    const regularNames = (pod.spec?.containers || []).map((container) => container.name);
    const initStatuses = statusByName(pod.status?.initContainerStatuses);
    const regularStatuses = statusByName(pod.status?.containerStatuses);

    const runningRegular = regularNames.filter((name) => hasRunningState(regularStatuses.get(name)));
    const runningInit = initNames.filter((name) => hasRunningState(initStatuses.get(name)));

    return unique([
        ...runningRegular,
        ...runningInit,
        ...regularNames,
        ...initNames,
    ]);
};

const podScore = (pod: K8sPod): number => {
    if (pod.metadata?.deletionTimestamp) return -1;

    const regularStatuses = pod.status?.containerStatuses || [];
    const initStatuses = pod.status?.initContainerStatuses || [];

    if (regularStatuses.some(hasRunningState)) return 4;
    if (initStatuses.some(hasRunningState)) return 3;
    if (pod.status?.phase === 'Running') return 2;
    if (pod.status?.phase === 'Pending') return 1;

    return 0;
};

export const resolveLogTargetFromPods = (
    namespace: string,
    pods: K8sPod[],
    ownerName = '',
): ResolvedLogTarget | null => {
    if (pods.length === 0) return null;

    const sortedPods = [...pods].sort((a, b) => podScore(b) - podScore(a));
    const selectedPod = sortedPods[0];
    const podContainerMap: Record<string, string[]> = {};

    for (const pod of pods) {
        podContainerMap[pod.metadata.name] = getLogContainerNames(pod);
    }

    return {
        namespace,
        pod: selectedPod.metadata.name,
        containers: podContainerMap[selectedPod.metadata.name] || [],
        siblingPods: sortedPods.map((pod) => pod.metadata.name),
        podContainerMap,
        ownerName,
        podCreationTime: selectedPod.metadata.creationTimestamp || '',
    };
};
