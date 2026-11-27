// Pure helpers shared by the workload lists (replica sets, jobs, daemon sets): the controller
// owner reference and the pod template reads. The List components and the golden-parity
// generator both import them, so the Go row specs are proven against the code that really runs.

export interface WorkloadController {
    kind: string;
    name: string;
    uid: string;
    apiVersion: string;
}

/** The owner reference marked as controller, or null. */
export function getController(item: any): WorkloadController | null {
    const owners = item.metadata?.ownerReferences || [];
    const controller = owners.find((owner: any) => owner.controller);
    return controller ? { kind: controller.kind, name: controller.name, uid: controller.uid, apiVersion: controller.apiVersion } : null;
}

/**
 * The controller as one JSON string: the "Controlled By" cell must carry the kind alone (its sort
 * and filter value), while the view's link needs the whole reference. Empty without one.
 */
export function controllerRefText(item: any): string {
    const controller = getController(item);
    return controller ? JSON.stringify(controller) : '';
}

/** Pod template containers of an object whose template lives at spec.template. */
export const templateContainers = (item: any): any[] => item.spec?.template?.spec?.containers || [];

/** First container image, '' without containers (the Image column's sort and filter value). */
export const firstImage = (containers: any[]): string => containers?.[0]?.image || '';

/** Every container image, one per line (display only; an image never contains a newline). */
export const imageLines = (containers: any[]): string => containers.map((c: any) => c.image).join('\n');

/** matchLabels (or any string map) as "k=v" pairs joined with a comma, in JavaScript key order. */
export const labelPairs = (labels: Record<string, string> | undefined): string =>
    Object.entries(labels || {}).map(([k, v]) => `${k}=${v}`).join(',');
