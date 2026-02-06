import React from 'react';

/**
 * Get status display info for a container
 */
function getStatusInfo(status: any) {
    if (!status) return { label: 'Unknown', color: 'text-gray-400' };

    if (status.state?.running) {
        return { label: 'Running', color: 'text-green-400' };
    }
    if (status.state?.waiting) {
        const reason = status.state.waiting.reason || 'Waiting';
        return { label: reason, color: 'text-yellow-400' };
    }
    if (status.state?.terminated) {
        const reason = status.state.terminated.reason || 'Terminated';
        return { label: reason, color: 'text-red-400' };
    }
    return { label: 'Unknown', color: 'text-gray-400' };
}

/**
 * Inline container selector component for use in tabs.
 * Similar to NodeShellTab's image selector pattern.
 *
 * @param {Array} containers - Array of container objects: { name, status?, isInit? }
 *                            or array of strings (container names) for backwards compatibility
 */
export default function ContainerSelector({
    containers,
    podName,
    title = 'Select Container',
    description,
    onSelect
}: any) {
    return (
        <div className="h-full w-full bg-background overflow-auto">
            <div className="max-w-md w-full p-6 mx-auto">
                <h3 className="text-lg font-medium text-foreground mb-4">{title}</h3>
                <p className="text-sm text-muted-foreground mb-4">
                    {description || (
                        <>Choose a container in <span className="font-medium text-foreground">{podName}</span></>
                    )}
                </p>

                <div className="flex flex-col gap-2">
                    {containers.map((container: any) => {
                        // Support both string and object format
                        const isObject = typeof container === 'object';
                        const name = isObject ? container.name : container;
                        const status = isObject ? container.status : null;
                        const isInit = isObject ? container.isInit : false;
                        const statusInfo = getStatusInfo(status);

                        return (
                            <button
                                key={name}
                                onClick={() => onSelect(name)}
                                className="w-full text-left px-4 py-3 rounded-md text-sm transition-colors bg-muted hover:bg-muted/80 text-foreground border border-border hover:border-blue-500 flex items-center justify-between"
                            >
                                <span className="flex items-center gap-2">
                                    {name}
                                    {isInit && <span className="text-xs text-yellow-400">(init)</span>}
                                </span>
                                {status && (
                                    <span className={`text-xs ${statusInfo.color}`}>
                                        {statusInfo.label}
                                    </span>
                                )}
                            </button>
                        );
                    })}
                </div>
            </div>
        </div>
    );
}
