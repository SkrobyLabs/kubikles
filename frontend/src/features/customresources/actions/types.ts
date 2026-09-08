export interface ResourceRef {
    context: string;
    group: string;
    version: string;
    resource: string;
    namespace: string;
    name: string;
    uid: string;
}
export interface OperatorAction {
    id: string;
    label: string;
    description: string;
    modes: { id: string; label: string; selectTargets: boolean }[];
}
export interface ActionTarget extends ResourceRef {
    kind: string;
    pending: boolean;
}
export interface ActionPlan {
    requestId: string;
    fingerprint?: string;
    source: ResourceRef;
    actionId: string;
    mode: string;
    summary: string;
    targets: ActionTarget[];
}
export interface ActionResult {
    message?: string;
    target: ActionTarget;
    status: 'requested' | 'pending' | 'failed';
    error?: string;
}
