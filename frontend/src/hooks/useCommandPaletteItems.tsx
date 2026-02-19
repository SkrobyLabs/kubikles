import { useEffect, useMemo } from 'react';
import { useK8s, useConfig } from '../context';
import {
    ALL_MENU_ITEMS,
    DEFAULT_MENU_SECTIONS,
    reconcileLayout,
    getVisibleItemIds,
    type SidebarLayoutSection,
} from '~/constants/menuStructure';

export interface CommandPaletteItem {
    id: string;
    label: string;
    path: string;
    viewId: string;
    type: 'builtin' | 'crd';
    group?: string;
}

interface UseCommandPaletteItemsResult {
    items: CommandPaletteItem[];
    loading: boolean;
    error: Error | null;
}

export const useCommandPaletteItems = (): UseCommandPaletteItemsResult => {
    const { currentContext, crds, crdsLoading, ensureCRDsLoaded } = useK8s();
    const { config } = useConfig();

    // Trigger CRD loading when context is available
    useEffect(() => {
        if (currentContext) {
            ensureCRDsLoaded();
        }
    }, [currentContext, ensureCRDsLoaded]);

    // Compute effective layout and visible items
    const sidebarLayout = config?.ui?.sidebar?.layout;

    // Build flat list of all navigable items
    const items = useMemo((): CommandPaletteItem[] => {
        const result: CommandPaletteItem[] = [];

        // Get effective sections for path building and visibility
        const sections = sidebarLayout
            ? reconcileLayout(sidebarLayout).sections
            : DEFAULT_MENU_SECTIONS.map((s: any) => ({ id: s.id, title: s.title, items: [...s.items] } as SidebarLayoutSection));

        // Only show items that are visible in the current layout
        const visibleIds = getVisibleItemIds(sections);

        // Build lookups for section title and custom item labels
        const itemToSection = new Map<string, string>();
        const itemToLabel = new Map<string, string>();
        for (const section of sections) {
            for (const itemId of section.items) {
                itemToSection.set(itemId, section.title);
                if (section.itemLabels?.[itemId]) {
                    itemToLabel.set(itemId, section.itemLabels[itemId]);
                }
            }
        }

        // Add static menu items (only visible ones)
        for (const [itemId, def] of Object.entries(ALL_MENU_ITEMS)) {
            if (itemId === 'crds') continue; // handled separately below
            if (!visibleIds.has(itemId)) continue;

            const label = itemToLabel.get(itemId) || def.label;
            const sectionTitle = itemToSection.get(itemId) || def.defaultSection;
            result.push({
                id: itemId,
                label,
                path: `${sectionTitle} > ${label}`,
                viewId: itemId,
                type: 'builtin'
            });
        }

        // Add CRD definitions link (always visible, part of Custom Resources)
        result.push({
            id: 'crds',
            label: 'Definitions',
            path: 'Custom Resources > Definitions',
            viewId: 'crds',
            type: 'builtin'
        });

        // Add CRD instances grouped by API group
        for (const crd of crds) {
            const group = crd.spec?.group || 'unknown';
            const kind = crd.spec?.names?.kind;
            const plural = crd.spec?.names?.plural;
            const versions = crd.spec?.versions || [];
            const storageVersion = versions.find((v: any) => v.storage)?.name || versions[0]?.name || 'v1';
            const namespaced = crd.spec?.scope === 'Namespaced';

            if (kind && plural) {
                const viewId = `cr:${group}:${storageVersion}:${plural}:${kind}:${namespaced}`;
                result.push({
                    id: `cr-${group}-${kind}`,
                    label: kind,
                    path: `Custom Resources > ${group} > ${kind}`,
                    viewId: viewId,
                    type: 'crd',
                    group: group
                });
            }
        }

        return result;
    }, [crds, sidebarLayout]);

    return { items, loading: crdsLoading, error: null };
};
