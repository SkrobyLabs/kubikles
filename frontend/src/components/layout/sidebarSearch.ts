interface SidebarSearchItem {
    id: string;
    label: string;
}

interface SidebarSearchGroup<T extends SidebarSearchItem = SidebarSearchItem> {
    id: string;
    title: string;
    items: T[];
}

interface CRDSearchItem {
    kind?: string;
    plural?: string;
}

export const matchesSidebarSearch = (query: string, ...values: Array<string | undefined>): boolean => {
    const normalizedQuery = query.trim().toLocaleLowerCase();
    if (!normalizedQuery) return true;

    return values.some(value => value?.toLocaleLowerCase().includes(normalizedQuery));
};

export const filterSidebarGroups = <T extends SidebarSearchItem>(
    groups: SidebarSearchGroup<T>[],
    query: string,
): SidebarSearchGroup<T>[] => {
    if (!query.trim()) return groups;

    return groups.flatMap(group => {
        if (matchesSidebarSearch(query, group.title)) {
            return [group];
        }

        const matchingItems = group.items.filter(item => matchesSidebarSearch(query, item.label, item.id));
        return matchingItems.length > 0 ? [{ ...group, items: matchingItems }] : [];
    });
};

export const filterCRDGroups = (
    groups: Record<string, CRDSearchItem[]>,
    query: string,
    includeAll: boolean = false,
): Record<string, CRDSearchItem[]> => {
    if (!query.trim() || includeAll) return groups;

    return Object.fromEntries(
        Object.entries(groups).flatMap(([groupName, resources]) => {
            if (matchesSidebarSearch(query, groupName)) {
                return [[groupName, resources]];
            }

            const matchingResources = resources.filter(resource =>
                matchesSidebarSearch(query, resource.kind, resource.plural)
            );
            return matchingResources.length > 0 ? [[groupName, matchingResources]] : [];
        })
    );
};
