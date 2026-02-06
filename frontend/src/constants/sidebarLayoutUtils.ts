/**
 * Pure utility functions for sidebar layout manipulation.
 * Extracted from SidebarLayoutEditor.tsx for testability.
 */
import {
    ALL_MENU_ITEMS,
    DEFAULT_MENU_SECTIONS,
    FIXED_SECTION_IDS,
    getDefaultLayout,
    type SidebarLayoutSection,
} from './menuStructure';

/** Remove an item from a section, cleaning up its custom label */
export function removeItemFromLayout(
    layout: SidebarLayoutSection[],
    sectionIdx: number,
    itemId: string,
): SidebarLayoutSection[] {
    return layout.map((s, i) => {
        if (i !== sectionIdx) return s;
        const items = s.items.filter(id => id !== itemId);
        if (!s.itemLabels?.[itemId]) return { ...s, items };
        const labels = { ...s.itemLabels };
        delete labels[itemId];
        return { ...s, items, itemLabels: Object.keys(labels).length > 0 ? labels : undefined };
    });
}

/** Rename an item within a section (stores in itemLabels, clears if matches original) */
export function renameItemInLayout(
    layout: SidebarLayoutSection[],
    sectionIdx: number,
    itemId: string,
    newLabel: string,
): SidebarLayoutSection[] {
    const originalLabel = ALL_MENU_ITEMS[itemId]?.label;
    return layout.map((s, i) => {
        if (i !== sectionIdx) return s;
        const labels = { ...(s.itemLabels || {}) };
        if (!newLabel || newLabel === originalLabel) {
            delete labels[itemId];
        } else {
            labels[itemId] = newLabel;
        }
        return { ...s, itemLabels: Object.keys(labels).length > 0 ? labels : undefined };
    });
}

/** Add an item to a section (appends to end) */
export function addItemToLayout(
    layout: SidebarLayoutSection[],
    itemId: string,
    sectionIdx?: number,
): SidebarLayoutSection[] {
    const targetIdx = sectionIdx ?? layout.findIndex(s => !FIXED_SECTION_IDS.has(s.id));
    if (targetIdx === -1 || layout.length === 0) return layout;
    return layout.map((s, i) =>
        i === targetIdx ? { ...s, items: [...s.items, itemId] } : s
    );
}

/** Add a group of items: to existing section or create new one from defaults */
export function addGroupToLayout(
    layout: SidebarLayoutSection[],
    sectionId: string,
    itemIds: string[],
): SidebarLayoutSection[] {
    const existingIdx = layout.findIndex(s => s.id === sectionId);
    if (existingIdx !== -1) {
        return layout.map((s, i) =>
            i === existingIdx ? { ...s, items: [...s.items, ...itemIds] } : s
        );
    }
    const defaultSec = DEFAULT_MENU_SECTIONS.find(s => s.id === sectionId);
    const newSection: SidebarLayoutSection = {
        id: sectionId,
        title: defaultSec?.title || sectionId,
        items: itemIds,
    };
    return [...layout, newSection];
}

/** Delete a section (guarded against fixed sections) */
export function deleteSectionFromLayout(
    layout: SidebarLayoutSection[],
    sectionIdx: number,
): SidebarLayoutSection[] {
    if (FIXED_SECTION_IDS.has(layout[sectionIdx].id)) return layout;
    return layout.filter((_, i) => i !== sectionIdx);
}

/** Rename a section */
export function renameSectionInLayout(
    layout: SidebarLayoutSection[],
    sectionIdx: number,
    newTitle: string,
): SidebarLayoutSection[] {
    return layout.map((s, i) => i === sectionIdx ? { ...s, title: newTitle } : s);
}

/** Check if a layout matches the defaults (for deciding undefined vs value) */
export function isDefaultLayout(layout: SidebarLayoutSection[]): boolean {
    const defaults = getDefaultLayout();
    return defaults.length === layout.length &&
        defaults.every((defSection, i) => {
            const section = layout[i];
            return section.id === defSection.id &&
                section.title === defSection.title &&
                !section.isCustom &&
                (!section.itemLabels || Object.keys(section.itemLabels).length === 0) &&
                section.items.length === defSection.items.length &&
                section.items.every((item, j) => item === defSection.items[j]);
        });
}

/** Compute available (hidden) items grouped by their default section */
export function getAvailableGroups(
    layout: SidebarLayoutSection[],
): { sectionId: string; title: string; items: { id: string; label: string }[] }[] {
    const visibleIds = new Set<string>();
    for (const section of layout) {
        for (const itemId of section.items) visibleIds.add(itemId);
    }

    const hiddenIds = Object.keys(ALL_MENU_ITEMS)
        .filter(id => id !== 'crds' && !visibleIds.has(id));
    if (hiddenIds.length === 0) return [];

    // Group by default section, preserving default section order
    const bySection = new Map<string, { id: string; label: string }[]>();
    for (const id of hiddenIds) {
        const def = ALL_MENU_ITEMS[id];
        const sectionId = def.defaultSection;
        if (!bySection.has(sectionId)) bySection.set(sectionId, []);
        bySection.get(sectionId)!.push({ id, label: def.label });
    }

    return DEFAULT_MENU_SECTIONS
        .filter(s => bySection.has(s.id))
        .map(s => ({ sectionId: s.id, title: s.title, items: bySection.get(s.id)! }));
}
