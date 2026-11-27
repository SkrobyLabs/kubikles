import React from 'react';
import { getContainerStatusColor, getPodStatusColor } from '~/utils/k8s-helpers';
import { getOwnerViewId } from '~/utils/owner-navigation';
import { useStoreValue, type ValueStore } from '~/utils/valueStore';
import type { ContainerStateCell } from './semantics';

/** One square per container status, coloured by its state. */
export const ContainerSquares = React.memo(function ContainerSquares({ statuses }: { statuses: any[] }) {
    return (
        <div className="flex gap-1">
            {statuses.map((status: any) => (
                <div
                    key={status.name}
                    className={`w-3 h-3 rounded-sm ${getContainerStatusColor(status)}`}
                    title={`${status.name}: ${Object.keys(status.state || {})[0]} (${status.state?.waiting?.reason || ''})`}
                />
            ))}
        </div>
    );
});

/** Container statuses rebuilt from the view's display-only containerStates cell (a JSON array of ContainerStateCell). */
export function statusesFromCell(text: string | null | undefined): any[] {
    if (!text) return [];
    let cells: ContainerStateCell[];
    try { cells = JSON.parse(text); } catch { return []; }
    return cells.map(c => ({
        name: c.n,
        state: c.s === 'running' ? { running: {} }
            : c.s === 'waiting' ? { waiting: { reason: c.w } }
            : c.s === 'terminated' ? { terminated: { exitCode: c.e } }
            : {},
    }));
}

export const PodStatusText = React.memo(function PodStatusText({ status }: { status: string }) {
    return <span className={`font-medium ${getPodStatusColor(status)}`}>{status}</span>;
});

export interface PodController {
    apiVersion?: string;
    kind: string;
    name: string;
    uid: string;
}

/** Owner link cell. CRDs arrive asynchronously, so it reads them from a store: when they load only these cells re-render. */
export const PodControllerLink = React.memo(function PodControllerLink({ controller, crdsStore, navigateWithSearch }: { controller: PodController | null; crdsStore: ValueStore<any>; navigateWithSearch: (viewId: string, search: string) => void }) {
    const crds = useStoreValue(crdsStore, v => v);
    if (!controller) {
        return <span className="text-gray-600">-</span>;
    }

    const viewId = getOwnerViewId(controller, crds);

    if (viewId) {
        return (
            <button
                onClick={(e) => {
                    e.stopPropagation();
                    navigateWithSearch(viewId, `uid:"${controller.uid}"`);
                }}
                className="text-primary hover:text-primary/80 hover:underline transition-colors"
                title={`Go to ${controller.kind}: ${controller.name}`}
            >
                {controller.kind}
            </button>
        );
    }

    return (
        <span className="text-gray-400" title={controller.name}>
            {controller.kind}
        </span>
    );
});

/** The Image cell: the image of a single container, the count of several (all images in the tooltip). */
export function renderImages(images: string[]): React.ReactNode {
    if (images.length === 0) return '-';
    if (images.length === 1) return <span title={images[0]}>{images[0]?.split('/').pop()}</span>;
    return <span title={images.join('\n')}>{images.length} images</span>;
}
