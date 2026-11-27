export type CordonAction = 'cordon' | 'uncordon';

/**
 * Whether a bulk cordon or uncordon has anything to act on. The legacy list hides the button when
 * no selected node qualifies; a windowed list cannot know that before the objects are fetched, so
 * it checks here and the modal then lists every selected node, as the legacy one does.
 */
export function hasCordonTargets(nodes: any[], action: CordonAction): boolean {
    return nodes.some((n: any) => (action === 'cordon' ? !n.spec?.unschedulable : n.spec?.unschedulable === true));
}
