import React from 'react';
import { getOwnerViewId } from '~/utils/owner-navigation';
import type { WorkloadController } from './workloadSemantics';

/** The "Controlled By" cell: a link to the owning resource's list when one exists, plain text otherwise. */
export function renderControlledBy(
    controller: WorkloadController | null,
    crds: any,
    navigateWithSearch: (viewId: string, search: string) => void,
): React.ReactNode {
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
}

/** The controller a view row carries in its controllerRef cell (empty cell: none). */
export function parseControllerRef(cell: string | null | undefined): WorkloadController | null {
    if (!cell) return null;
    try {
        return JSON.parse(cell) as WorkloadController;
    } catch {
        return null;
    }
}
