import { describe, expect, it } from 'vitest';
import { filterCRDGroups, filterSidebarGroups, matchesSidebarSearch } from './sidebarSearch';

const groups = [
    {
        id: 'workloads',
        title: 'Workloads',
        items: [
            { id: 'pods', label: 'Pods' },
            { id: 'deployments', label: 'Deployments' },
        ],
    },
    {
        id: 'network',
        title: 'Network',
        items: [
            { id: 'services', label: 'Services' },
        ],
    },
];

describe('sidebar search', () => {
    it('matches case-insensitively and ignores surrounding whitespace', () => {
        expect(matchesSidebarSearch('  PoD ', 'Pods')).toBe(true);
        expect(matchesSidebarSearch('stateful', 'Pods')).toBe(false);
    });

    it('keeps all items when a section title matches', () => {
        expect(filterSidebarGroups(groups, 'workload')).toEqual([groups[0]]);
    });

    it('keeps only matching menu items within their sections', () => {
        expect(filterSidebarGroups(groups, 'deploy')).toEqual([
            {
                ...groups[0],
                items: [groups[0].items[1]],
            },
        ]);
    });

    it('matches custom resources by API group, kind, or plural name', () => {
        const crdGroups = {
            'apps.example.io': [
                { kind: 'Widget', plural: 'widgets' },
                { kind: 'Gadget', plural: 'gadgets' },
            ],
            'infra.example.io': [
                { kind: 'Database', plural: 'databases' },
            ],
        };

        expect(filterCRDGroups(crdGroups, 'apps')).toEqual({
            'apps.example.io': crdGroups['apps.example.io'],
        });
        expect(filterCRDGroups(crdGroups, 'database')).toEqual({
            'infra.example.io': crdGroups['infra.example.io'],
        });
    });
});
