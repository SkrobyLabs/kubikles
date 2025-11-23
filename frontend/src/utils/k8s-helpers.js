export const getPodStatus = (pod) => {
    if (pod.metadata?.deletionTimestamp) return 'Terminating';

    // Check for init container failures
    if (pod.status?.initContainerStatuses) {
        for (const status of pod.status.initContainerStatuses) {
            if (status.state?.terminated && status.state.terminated.exitCode !== 0) {
                return 'Init:Error';
            }
            if (status.state?.waiting && status.state.waiting.reason === 'CrashLoopBackOff') {
                return 'Init:CrashLoopBackOff';
            }
            if (status.state?.running === undefined && status.state?.terminated === undefined) {
                // Init container is still running or waiting
                return 'Init:Running'; // Simplified
            }
        }
    }

    return pod.status?.phase || 'Unknown';
};

export const getPodStatusPriority = (status) => {
    switch (status) {
        case 'Failed': return 1;
        case 'CrashLoopBackOff': return 2;
        case 'ErrImagePull': return 3;
        case 'ImagePullBackOff': return 4;
        case 'Terminating': return 5;
        case 'Pending': return 6;
        case 'ContainerCreating': return 7;
        case 'Running': return 8;
        case 'Succeeded': return 9;
        default: return 10;
    }
};

export const getPodStatusColor = (status) => {
    switch (status) {
        case 'Running':
            return 'text-success';
        case 'Succeeded':
            return 'text-success/70'; // Dimmed green
        case 'Pending':
        case 'ContainerCreating':
            return 'text-warning'; // Orange
        case 'Terminating':
        case 'CrashLoopBackOff':
        case 'ImagePullBackOff':
        case 'ErrImagePull':
        case 'Unknown':
            return 'text-red-orange'; // Orange-red
        case 'Failed':
            return 'text-error'; // Red
        default:
            return 'text-text';
    }
};

export const getContainerStatusColor = (status) => {
    if (status.state?.running) return 'bg-success';
    if (status.state?.waiting) {
        const reason = status.state.waiting.reason;
        if (reason === 'CrashLoopBackOff' || reason === 'ErrImagePull' || reason === 'ImagePullBackOff') {
            return 'bg-red-orange';
        }
        return 'bg-warning';
    }
    if (status.state?.terminated) {
        return status.state.terminated.exitCode === 0 ? 'bg-success/50' : 'bg-error';
    }
    return 'bg-gray-500';
};

export const getEffectivePodStatus = (pod) => {
    // If pod is terminating, that's the status
    if (pod.metadata?.deletionTimestamp) return 'Terminating';

    const containerStatuses = pod.status?.containerStatuses || [];

    // If multiple containers, ignore Succeeded ones (unless all are succeeded)
    let statusesToCheck = containerStatuses;
    if (containerStatuses.length > 1) {
        const nonSucceeded = containerStatuses.filter(s =>
            !(s.state?.terminated && s.state.terminated.exitCode === 0)
        );
        if (nonSucceeded.length > 0) {
            statusesToCheck = nonSucceeded;
        }
    }

    // Find worst status among relevant containers
    let worstStatus = null;
    let worstPriority = -1;

    // Reuse the severity logic from getPodStatus
    const getStatusSeverity = (s) => {
        switch (s) {
            case 'Failed': return 100;
            case 'Terminating': return 90;
            case 'ErrImagePull': return 80;
            case 'CrashLoopBackOff': return 70;
            case 'ImagePullBackOff': return 60;
            case 'ContainerCreating': return 50;
            case 'Pending': return 40;
            case 'Running': return 30;
            case 'Succeeded': return 20;
            default: return 0;
        }
    };

    for (const status of statusesToCheck) {
        let currentStatus = null;
        if (status.state?.waiting) {
            currentStatus = status.state.waiting.reason;
        } else if (status.state?.terminated) {
            currentStatus = status.state.terminated.exitCode === 0 ? 'Succeeded' : 'Failed';
        } else if (status.state?.running) {
            currentStatus = 'Running';
        }

        if (currentStatus) {
            const priority = getStatusSeverity(currentStatus);
            if (priority > worstPriority) {
                worstPriority = priority;
                worstStatus = currentStatus;
            }
        }
    }

    if (worstStatus) return worstStatus;

    return pod.status?.phase || 'Unknown';
};

export const getDeploymentPods = (deployment, allPods) => {
    if (!deployment.spec?.selector?.matchLabels) return [];
    const selector = deployment.spec.selector.matchLabels;
    return (allPods || []).filter(pod => {
        if (pod.metadata.namespace !== deployment.metadata.namespace) return false;
        for (const [key, value] of Object.entries(selector)) {
            if (pod.metadata.labels?.[key] !== value) return false;
        }
        return true;
    });
};
