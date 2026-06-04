export function parseCpuQuantity(q: string | undefined): number {
    if (!q) return 0;
    let value: number;
    if (q.endsWith('n')) value = parseInt(q, 10) / 1e6;
    else if (q.endsWith('u')) value = parseInt(q, 10) / 1e3;
    else if (q.endsWith('m')) value = parseInt(q, 10);
    else value = parseFloat(q) * 1000;
    return Number.isFinite(value) ? value : 0;
}

export function parseMemoryQuantity(q: string | undefined): number {
    if (!q) return 0;
    const units: [string, number][] = [
        ['Ti', 1024 ** 4], ['Gi', 1024 ** 3], ['Mi', 1024 ** 2], ['Ki', 1024],
        ['T', 1e12], ['G', 1e9], ['M', 1e6], ['K', 1e3],
    ];
    for (const [suffix, multiplier] of units) {
        if (q.endsWith(suffix)) {
            const value = parseFloat(q.slice(0, -suffix.length)) * multiplier;
            return Number.isFinite(value) ? value : 0;
        }
    }
    const value = parseFloat(q);
    return Number.isFinite(value) ? value : 0;
}

export function getPodResourceRequests(pod: any): { cpuMillis: number; memBytes: number } {
    const podCpuMillis = parseCpuQuantity(pod.spec?.resources?.requests?.cpu);
    const podMemBytes = parseMemoryQuantity(pod.spec?.resources?.requests?.memory);

    let containerAndSidecarCpuMillis = 0;
    let containerAndSidecarMemBytes = 0;
    for (const container of pod.spec?.containers || []) {
        containerAndSidecarCpuMillis += parseCpuQuantity(container.resources?.requests?.cpu);
        containerAndSidecarMemBytes += parseMemoryQuantity(container.resources?.requests?.memory);
    }

    let initCpuMillis = 0;
    let initMemBytes = 0;
    let restartableInitCpuMillis = 0;
    let restartableInitMemBytes = 0;
    for (const container of pod.spec?.initContainers || []) {
        let cpuMillis = parseCpuQuantity(container.resources?.requests?.cpu);
        let memBytes = parseMemoryQuantity(container.resources?.requests?.memory);
        if (container.restartPolicy === 'Always') {
            containerAndSidecarCpuMillis += cpuMillis;
            containerAndSidecarMemBytes += memBytes;
            restartableInitCpuMillis += cpuMillis;
            restartableInitMemBytes += memBytes;
            cpuMillis = restartableInitCpuMillis;
            memBytes = restartableInitMemBytes;
        } else {
            cpuMillis += restartableInitCpuMillis;
            memBytes += restartableInitMemBytes;
        }
        initCpuMillis = Math.max(initCpuMillis, cpuMillis);
        initMemBytes = Math.max(initMemBytes, memBytes);
    }

    return {
        cpuMillis: Math.max(podCpuMillis, containerAndSidecarCpuMillis, initCpuMillis) + parseCpuQuantity(pod.spec?.overhead?.cpu),
        memBytes: Math.max(podMemBytes, containerAndSidecarMemBytes, initMemBytes) + parseMemoryQuantity(pod.spec?.overhead?.memory),
    };
}

export function hasResourceValues(resources: any): boolean {
    return !!resources && Object.values(resources).some((value) => value != null && value !== '');
}
