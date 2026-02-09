import { describe, it, expect } from 'vitest';
import {
    ALL_MENU_ITEMS,
    DEFAULT_MENU_SECTIONS,
    FIXED_SECTION_IDS,
    reconcileLayout,
    getDefaultLayout,
    getVisibleItemIds,
} from './menuStructure';

describe('reconcileLayout', () => {
    it('prunes items no longer in the ALL_MENU_ITEMS registry', () => {
        const layout = [
            { id: 'workloads', title: 'Workloads', items: ['pods', 'nonexistent-item', 'deployments'] },
        ];
        const { sections: result } = reconcileLayout(layout);
        const workloads = result.find(s => s.id === 'workloads');
        // nonexistent-item should be removed, pods and deployments kept
        expect(workloads.items).toContain('pods');
        expect(workloads.items).toContain('deployments');
        expect(workloads.items).not.toContain('nonexistent-item');
    });

    it('keeps sections with remaining items after pruning', () => {
        const layout = [
            { id: 'workloads', title: 'Workloads', items: ['pods'] },
            { id: 'cluster', title: 'Cluster', items: ['nonexistent'] },
        ];
        const { sections: result } = reconcileLayout(layout);
        // workloads stays (has items), cluster removed (no items and not custom/fixed)
        const sectionIds = result.map(s => s.id);
        expect(sectionIds).toContain('workloads');
        expect(sectionIds).not.toContain('cluster');
    });

    it('removes empty sections that are not custom and not fixed', () => {
        const layout = [
            { id: 'workloads', title: 'Workloads', items: ['nonexistent'] },
        ];
        const { sections: result } = reconcileLayout(layout);
        const nonFixed = result.filter(s => !FIXED_SECTION_IDS.has(s.id));
        expect(nonFixed.filter(s => s.id === 'workloads')).toHaveLength(0);
    });

    it('preserves empty custom sections (isCustom: true)', () => {
        const layout = [
            { id: 'my-custom', title: 'My Custom', items: [], isCustom: true },
        ];
        const { sections: result } = reconcileLayout(layout);
        expect(result.find(s => s.id === 'my-custom')).toBeDefined();
    });

    it('preserves fixed sections even when empty', () => {
        const layout = [
            { id: 'custom-resources', title: 'Custom Resources', items: [] },
        ];
        const { sections: result } = reconcileLayout(layout);
        expect(result.find(s => s.id === 'custom-resources')).toBeDefined();
    });

    it('re-adds missing fixed sections from defaults', () => {
        const layout = [
            { id: 'workloads', title: 'Workloads', items: ['pods'] },
        ];
        const { sections: result } = reconcileLayout(layout);
        expect(result.find(s => s.id === 'custom-resources')).toBeDefined();
    });

    it('preserves section order from stored layout', () => {
        const layout = [
            { id: 'network', title: 'Network', items: ['services'] },
            { id: 'workloads', title: 'Workloads', items: ['pods'] },
        ];
        const { sections: result } = reconcileLayout(layout);
        const networkIdx = result.findIndex(s => s.id === 'network');
        const workloadsIdx = result.findIndex(s => s.id === 'workloads');
        expect(networkIdx).toBeLessThan(workloadsIdx);
    });

    it('auto-injects new items not present in stored layout', () => {
        // Layout has only 'pods' — all other items that were never in the stored layout
        // should be auto-injected into their default sections
        const layout = [
            { id: 'workloads', title: 'Workloads', items: ['pods'] },
        ];
        const { sections: result } = reconcileLayout(layout);
        const workloads = result.find(s => s.id === 'workloads');
        // 'deployments' was never in stored layout, should be auto-injected
        expect(workloads.items).toContain('deployments');
    });

    it('does not re-inject items that exist in stored layout sections', () => {
        // 'pods' is already present in the stored layout, so it should NOT be
        // duplicated into its default section by auto-injection
        const layout = getDefaultLayout();
        const { sections: result } = reconcileLayout(layout);
        const workloads = result.find(s => s.id === 'workloads');
        const podCount = workloads.items.filter(id => id === 'pods').length;
        expect(podCount).toBe(1);
    });

    it('preserves itemLabels on sections', () => {
        const layout = [
            { id: 'workloads', title: 'Workloads', items: ['pods'], itemLabels: { pods: 'My Pods' } },
        ];
        const { sections: result } = reconcileLayout(layout);
        const section = result.find(s => s.id === 'workloads');
        expect(section.itemLabels).toEqual({ pods: 'My Pods' });
    });
});

describe('getDefaultLayout', () => {
    it('returns one section per DEFAULT_MENU_SECTIONS entry', () => {
        const layout = getDefaultLayout();
        expect(layout.length).toBe(DEFAULT_MENU_SECTIONS.length);
    });

    it('returns fresh arrays (not references to DEFAULT_MENU_SECTIONS)', () => {
        const layout = getDefaultLayout();
        layout[0].items.push('extra');
        const fresh = getDefaultLayout();
        expect(fresh[0].items).not.toContain('extra');
    });

    it('includes all default section IDs', () => {
        const layout = getDefaultLayout();
        const ids = layout.map(s => s.id);
        for (const section of DEFAULT_MENU_SECTIONS) {
            expect(ids).toContain(section.id);
        }
    });
});

describe('getVisibleItemIds', () => {
    it('returns empty set for empty layout', () => {
        const result = getVisibleItemIds([]);
        expect(result.size).toBe(0);
    });

    it('collects items from all sections', () => {
        const layout = [
            { id: 'a', title: 'A', items: ['pods', 'nodes'] },
            { id: 'b', title: 'B', items: ['services'] },
        ];
        const result = getVisibleItemIds(layout);
        expect(result.has('pods')).toBe(true);
        expect(result.has('nodes')).toBe(true);
        expect(result.has('services')).toBe(true);
    });

    it('deduplicates items across sections', () => {
        const layout = [
            { id: 'a', title: 'A', items: ['pods'] },
            { id: 'b', title: 'B', items: ['pods'] },
        ];
        const result = getVisibleItemIds(layout);
        expect(result.size).toBe(1);
    });
});

describe('data integrity', () => {
    it('every item in DEFAULT_MENU_SECTIONS exists in ALL_MENU_ITEMS', () => {
        for (const section of DEFAULT_MENU_SECTIONS) {
            for (const itemId of section.items) {
                expect(ALL_MENU_ITEMS[itemId], `Missing ALL_MENU_ITEMS entry for "${itemId}" in section "${section.id}"`).toBeDefined();
            }
        }
    });

    it('every item in ALL_MENU_ITEMS has a defaultSection matching a section in DEFAULT_MENU_SECTIONS', () => {
        const sectionIds = new Set(DEFAULT_MENU_SECTIONS.map(s => s.id));
        for (const [itemId, def] of Object.entries(ALL_MENU_ITEMS)) {
            expect(sectionIds.has(def.defaultSection), `Item "${itemId}" has defaultSection "${def.defaultSection}" which doesn't match any section`).toBe(true);
        }
    });

    it('FIXED_SECTION_IDS are all present in DEFAULT_MENU_SECTIONS', () => {
        const sectionIds = new Set(DEFAULT_MENU_SECTIONS.map(s => s.id));
        for (const fixedId of FIXED_SECTION_IDS) {
            expect(sectionIds.has(fixedId), `Fixed section "${fixedId}" not found in DEFAULT_MENU_SECTIONS`).toBe(true);
        }
    });
});
