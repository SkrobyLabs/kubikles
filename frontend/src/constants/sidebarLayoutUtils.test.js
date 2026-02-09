import { describe, it, expect } from 'vitest';
import {
    removeItemFromLayout,
    renameItemInLayout,
    addItemToLayout,
    addGroupToLayout,
    deleteSectionFromLayout,
    renameSectionInLayout,
    isDefaultLayout,
    getAvailableGroups,
} from './sidebarLayoutUtils';
import { getDefaultLayout, ALL_MENU_ITEMS, DEFAULT_MENU_SECTIONS, reconcileLayout } from './menuStructure';

// Helper: create a minimal layout for tests
function makeLayout(sections) {
    return sections.map(s => ({
        id: s.id,
        title: s.title,
        items: [...s.items],
        ...(s.isCustom ? { isCustom: true } : {}),
        ...(s.itemLabels ? { itemLabels: { ...s.itemLabels } } : {}),
    }));
}

describe('removeItemFromLayout', () => {
    it('removes item from the specified section', () => {
        const layout = makeLayout([{ id: 'a', title: 'A', items: ['pods', 'nodes'] }]);
        const result = removeItemFromLayout(layout, 0, 'pods');
        expect(result[0].items).toEqual(['nodes']);
    });

    it('cleans up custom label when item is removed', () => {
        const layout = makeLayout([{
            id: 'a', title: 'A', items: ['pods', 'nodes'],
            itemLabels: { pods: 'My Pods' },
        }]);
        const result = removeItemFromLayout(layout, 0, 'pods');
        expect(result[0].itemLabels).toBeUndefined();
    });

    it('preserves other items in the section', () => {
        const layout = makeLayout([{ id: 'a', title: 'A', items: ['pods', 'nodes', 'events'] }]);
        const result = removeItemFromLayout(layout, 0, 'nodes');
        expect(result[0].items).toEqual(['pods', 'events']);
    });

    it('preserves other sections unchanged', () => {
        const layout = makeLayout([
            { id: 'a', title: 'A', items: ['pods'] },
            { id: 'b', title: 'B', items: ['services'] },
        ]);
        const result = removeItemFromLayout(layout, 0, 'pods');
        expect(result[1].items).toEqual(['services']);
    });
});

describe('renameItemInLayout', () => {
    it('stores custom label in itemLabels', () => {
        const layout = makeLayout([{ id: 'a', title: 'A', items: ['pods'] }]);
        const result = renameItemInLayout(layout, 0, 'pods', 'Custom Pods');
        expect(result[0].itemLabels).toEqual({ pods: 'Custom Pods' });
    });

    it('clears custom label when renamed back to original', () => {
        const originalLabel = ALL_MENU_ITEMS['pods'].label;
        const layout = makeLayout([{
            id: 'a', title: 'A', items: ['pods'],
            itemLabels: { pods: 'Custom Pods' },
        }]);
        const result = renameItemInLayout(layout, 0, 'pods', originalLabel);
        expect(result[0].itemLabels).toBeUndefined();
    });

    it('clears itemLabels entirely when last custom label is removed', () => {
        const originalLabel = ALL_MENU_ITEMS['pods'].label;
        const layout = makeLayout([{
            id: 'a', title: 'A', items: ['pods'],
            itemLabels: { pods: 'Custom' },
        }]);
        const result = renameItemInLayout(layout, 0, 'pods', originalLabel);
        expect(result[0].itemLabels).toBeUndefined();
    });

    it('preserves existing labels for other items', () => {
        const layout = makeLayout([{
            id: 'a', title: 'A', items: ['pods', 'nodes'],
            itemLabels: { pods: 'Custom Pods' },
        }]);
        const result = renameItemInLayout(layout, 0, 'nodes', 'Custom Nodes');
        expect(result[0].itemLabels).toEqual({ pods: 'Custom Pods', nodes: 'Custom Nodes' });
    });
});

describe('addItemToLayout', () => {
    it('appends item to specified section', () => {
        const layout = makeLayout([{ id: 'a', title: 'A', items: ['pods'] }]);
        const result = addItemToLayout(layout, 'nodes', 0);
        expect(result[0].items).toEqual(['pods', 'nodes']);
    });

    it('defaults to first non-fixed section when no sectionIdx given', () => {
        const layout = makeLayout([
            { id: 'custom-resources', title: 'Custom Resources', items: ['crds'] },
            { id: 'workloads', title: 'Workloads', items: ['pods'] },
        ]);
        const result = addItemToLayout(layout, 'nodes');
        expect(result[1].items).toEqual(['pods', 'nodes']);
    });

    it('does not modify other sections', () => {
        const layout = makeLayout([
            { id: 'a', title: 'A', items: ['pods'] },
            { id: 'b', title: 'B', items: ['services'] },
        ]);
        const result = addItemToLayout(layout, 'nodes', 0);
        expect(result[1].items).toEqual(['services']);
    });
});

describe('addGroupToLayout', () => {
    it('adds items to existing section by id', () => {
        const layout = makeLayout([{ id: 'workloads', title: 'Workloads', items: ['pods'] }]);
        const result = addGroupToLayout(layout, 'workloads', ['jobs', 'cronjobs']);
        expect(result[0].items).toEqual(['pods', 'jobs', 'cronjobs']);
    });

    it('creates new section from defaults when section not in layout', () => {
        const layout = makeLayout([{ id: 'workloads', title: 'Workloads', items: ['pods'] }]);
        const result = addGroupToLayout(layout, 'network', ['services']);
        expect(result.length).toBe(2);
        expect(result[1].id).toBe('network');
        expect(result[1].items).toEqual(['services']);
    });

    it('uses section title from DEFAULT_MENU_SECTIONS for new section', () => {
        const layout = makeLayout([{ id: 'workloads', title: 'Workloads', items: ['pods'] }]);
        const result = addGroupToLayout(layout, 'network', ['services']);
        const networkDefault = DEFAULT_MENU_SECTIONS.find(s => s.id === 'network');
        expect(result[1].title).toBe(networkDefault.title);
    });
});

describe('deleteSectionFromLayout', () => {
    it('removes section at given index', () => {
        const layout = makeLayout([
            { id: 'a', title: 'A', items: ['pods'] },
            { id: 'b', title: 'B', items: ['services'] },
        ]);
        const result = deleteSectionFromLayout(layout, 0);
        expect(result.length).toBe(1);
        expect(result[0].id).toBe('b');
    });

    it('returns layout unchanged if section is fixed', () => {
        const layout = makeLayout([
            { id: 'custom-resources', title: 'Custom Resources', items: ['crds'] },
        ]);
        const result = deleteSectionFromLayout(layout, 0);
        expect(result.length).toBe(1);
        expect(result[0].id).toBe('custom-resources');
    });

    it('preserves other sections', () => {
        const layout = makeLayout([
            { id: 'a', title: 'A', items: ['pods'] },
            { id: 'b', title: 'B', items: ['services'] },
            { id: 'c', title: 'C', items: ['nodes'] },
        ]);
        const result = deleteSectionFromLayout(layout, 1);
        expect(result.map(s => s.id)).toEqual(['a', 'c']);
    });
});

describe('renameSectionInLayout', () => {
    it('updates title for the specified section', () => {
        const layout = makeLayout([{ id: 'a', title: 'A', items: ['pods'] }]);
        const result = renameSectionInLayout(layout, 0, 'New Title');
        expect(result[0].title).toBe('New Title');
    });

    it('preserves other sections unchanged', () => {
        const layout = makeLayout([
            { id: 'a', title: 'A', items: ['pods'] },
            { id: 'b', title: 'B', items: ['services'] },
        ]);
        const result = renameSectionInLayout(layout, 0, 'New A');
        expect(result[1].title).toBe('B');
    });
});

describe('isDefaultLayout', () => {
    it('returns true for exact default layout', () => {
        const layout = getDefaultLayout();
        expect(isDefaultLayout(layout)).toBe(true);
    });

    it('returns false when sections are reordered', () => {
        const layout = getDefaultLayout();
        // Swap first two sections
        [layout[0], layout[1]] = [layout[1], layout[0]];
        expect(isDefaultLayout(layout)).toBe(false);
    });

    it('returns false when items are reordered', () => {
        const layout = getDefaultLayout();
        // Find a section with at least 2 items
        const section = layout.find(s => s.items.length > 1);
        if (section) {
            [section.items[0], section.items[1]] = [section.items[1], section.items[0]];
        }
        expect(isDefaultLayout(layout)).toBe(false);
    });

    it('returns false when items are missing', () => {
        const layout = getDefaultLayout();
        layout[0].items.pop();
        expect(isDefaultLayout(layout)).toBe(false);
    });

    it('returns false when custom section is present', () => {
        const layout = getDefaultLayout();
        layout[0].isCustom = true;
        expect(isDefaultLayout(layout)).toBe(false);
    });

    it('returns false when section has custom title', () => {
        const layout = getDefaultLayout();
        layout[0].title = 'Custom Title';
        expect(isDefaultLayout(layout)).toBe(false);
    });

    it('returns false when section has itemLabels', () => {
        const layout = getDefaultLayout();
        layout[0].itemLabels = { [layout[0].items[0]]: 'Custom' };
        expect(isDefaultLayout(layout)).toBe(false);
    });

    it('returns false when excludedItems has entries', () => {
        const layout = getDefaultLayout();
        expect(isDefaultLayout(layout, ['pods'])).toBe(false);
    });

    it('returns true when excludedItems is empty array', () => {
        const layout = getDefaultLayout();
        expect(isDefaultLayout(layout, [])).toBe(true);
    });

    it('returns true when excludedItems is undefined', () => {
        const layout = getDefaultLayout();
        expect(isDefaultLayout(layout, undefined)).toBe(true);
    });
});

describe('getAvailableGroups', () => {
    it('returns empty array when all items are visible', () => {
        const layout = getDefaultLayout();
        const result = getAvailableGroups(layout);
        expect(result).toEqual([]);
    });

    it('groups hidden items by their default section', () => {
        // Layout with only pods visible - all others should be available
        const layout = [{ id: 'workloads', title: 'Workloads', items: ['pods'] }];
        const result = getAvailableGroups(layout);
        // Should have groups for sections that have hidden items
        expect(result.length).toBeGreaterThan(0);
        // The workloads group should have the rest of the workload items (not pods)
        const workloadsGroup = result.find(g => g.sectionId === 'workloads');
        if (workloadsGroup) {
            expect(workloadsGroup.items.find(i => i.id === 'pods')).toBeUndefined();
            expect(workloadsGroup.items.find(i => i.id === 'deployments')).toBeDefined();
        }
    });

    it('preserves DEFAULT_MENU_SECTIONS order', () => {
        const layout = []; // nothing visible
        const result = getAvailableGroups(layout);
        const sectionIds = result.map(g => g.sectionId);
        const defaultOrder = DEFAULT_MENU_SECTIONS.map(s => s.id);
        // Verify result order matches default order (filtered)
        let lastIdx = -1;
        for (const id of sectionIds) {
            const idx = defaultOrder.indexOf(id);
            expect(idx).toBeGreaterThan(lastIdx);
            lastIdx = idx;
        }
    });

    it('excludes crds from available items', () => {
        const layout = []; // nothing visible
        const result = getAvailableGroups(layout);
        for (const group of result) {
            expect(group.items.find(i => i.id === 'crds')).toBeUndefined();
        }
    });

    it('uses original labels (not custom) for available items', () => {
        const layout = [{ id: 'workloads', title: 'Workloads', items: ['pods'], itemLabels: { pods: 'Custom' } }];
        const result = getAvailableGroups(layout);
        // Deployments should use original label
        const workloadsGroup = result.find(g => g.sectionId === 'workloads');
        if (workloadsGroup) {
            const deploymentsItem = workloadsGroup.items.find(i => i.id === 'deployments');
            expect(deploymentsItem?.label).toBe(ALL_MENU_ITEMS['deployments'].label);
        }
    });
});

describe('reconcileLayout', () => {
    it('prunes items no longer in registry', () => {
        const layout = makeLayout([
            { id: 'workloads', title: 'Workloads', items: ['pods', 'nonexistent-item'] },
        ]);
        const { sections } = reconcileLayout(layout);
        expect(sections[0].items).not.toContain('nonexistent-item');
        expect(sections[0].items).toContain('pods');
    });

    it('does not re-inject excluded items', () => {
        // Layout with only pods — 'deployments' is excluded
        const layout = makeLayout([
            { id: 'workloads', title: 'Workloads', items: ['pods'] },
        ]);
        const { sections } = reconcileLayout(layout, ['deployments']);
        const workloads = sections.find(s => s.id === 'workloads');
        expect(workloads.items).not.toContain('deployments');
    });

    it('auto-injects truly new items (not in layout, not excluded)', () => {
        // All items except 'pods' are not in layout or excluded
        const allIds = Object.keys(ALL_MENU_ITEMS).filter(id => id !== 'crds');
        const layout = makeLayout([
            { id: 'workloads', title: 'Workloads', items: ['pods'] },
        ]);
        const { sections } = reconcileLayout(layout, []);
        const allResultItems = sections.flatMap(s => s.items);
        // deployments should have been auto-injected into its default section
        expect(allResultItems).toContain('deployments');
    });

    it('returns newExcluded for defaultHidden items', () => {
        // Temporarily make 'pods' defaultHidden to test
        const orig = ALL_MENU_ITEMS['pods'].defaultHidden;
        ALL_MENU_ITEMS['pods'].defaultHidden = true;
        try {
            // Layout without pods, pods not in excludedItems
            const layout = makeLayout([
                { id: 'workloads', title: 'Workloads', items: ['deployments'] },
            ]);
            const { sections, newExcluded } = reconcileLayout(layout, []);
            expect(newExcluded).toContain('pods');
            // pods should NOT be in any section
            const allResultItems = sections.flatMap(s => s.items);
            expect(allResultItems).not.toContain('pods');
        } finally {
            ALL_MENU_ITEMS['pods'].defaultHidden = orig;
        }
    });

    it('returns empty newExcluded when no new defaultHidden items', () => {
        const layout = getDefaultLayout();
        const { newExcluded } = reconcileLayout(layout, []);
        expect(newExcluded).toEqual([]);
    });

    it('ensures fixed sections are always present', () => {
        const layout = makeLayout([
            { id: 'workloads', title: 'Workloads', items: ['pods'] },
        ]);
        const { sections } = reconcileLayout(layout, []);
        expect(sections.find(s => s.id === 'custom-resources')).toBeDefined();
    });

    it('treats undefined excludedItems as empty array', () => {
        const layout = makeLayout([
            { id: 'workloads', title: 'Workloads', items: ['pods'] },
        ]);
        const withUndefined = reconcileLayout(layout, undefined);
        const withEmpty = reconcileLayout(layout, []);
        // Should produce same sections
        expect(withUndefined.sections.map(s => s.id)).toEqual(withEmpty.sections.map(s => s.id));
    });
});
