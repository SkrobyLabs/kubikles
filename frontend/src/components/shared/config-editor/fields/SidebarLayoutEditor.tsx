import React, { useState, useMemo, useCallback } from 'react';
import {
    DndContext,
    pointerWithin,
    rectIntersection,
    KeyboardSensor,
    PointerSensor,
    useSensor,
    useSensors,
    useDraggable,
    useDroppable,
    DragOverlay,
    type DragStartEvent,
    type DragEndEvent,
    type DragOverEvent,
    type CollisionDetection,
} from '@dnd-kit/core';
import {
    arrayMove,
    SortableContext,
    sortableKeyboardCoordinates,
    verticalListSortingStrategy,
    useSortable,
} from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import {
    Bars3Icon,
    ChevronDownIcon,
    ChevronRightIcon,
    PencilIcon,
    TrashIcon,
    PlusIcon,
    ArrowUturnLeftIcon,
    XMarkIcon,
} from '@heroicons/react/24/outline';
import {
    ALL_MENU_ITEMS,
    FIXED_SECTION_IDS,
    getDefaultLayout,
    reconcileLayout,
    type SidebarLayoutSection,
} from '~/constants/menuStructure';
import {
    removeItemFromLayout,
    renameItemInLayout,
    addItemToLayout,
    addGroupToLayout as addGroupToLayoutUtil,
    deleteSectionFromLayout,
    renameSectionInLayout,
    isDefaultLayout,
    getAvailableGroups,
} from '~/constants/sidebarLayoutUtils';
import { useConfig } from '~/context';

// ---- Insertion indicator line ----

function InsertionIndicator() {
    return (
        <div className="flex items-center gap-1 py-0.5">
            <div className="w-1.5 h-1.5 rounded-full bg-primary shrink-0" />
            <div className="flex-1 h-0.5 bg-primary rounded" />
        </div>
    );
}

// ---- Sortable item (single menu item within a section) ----

function SortableItem({ id, label, icon: Icon }: { id: string; label: string; icon?: React.ComponentType<any> }) {
    const {
        attributes,
        listeners,
        setNodeRef,
        transform,
        transition,
        isDragging,
    } = useSortable({ id, data: { type: 'item', source: 'layout' } });

    const style = {
        transform: CSS.Translate.toString(transform),
        transition,
        opacity: isDragging ? 0 : 1,
    };

    return (
        <div
            ref={setNodeRef}
            style={style}
            className="flex items-center gap-2 px-2 py-1.5 rounded text-sm text-gray-300 bg-surface hover:bg-white/5 group"
        >
            <button
                className="cursor-grab text-gray-500 hover:text-gray-300 shrink-0 touch-none"
                {...attributes}
                {...listeners}
            >
                <Bars3Icon className="w-3.5 h-3.5" />
            </button>
            {Icon && <Icon className="w-4 h-4 text-gray-400 shrink-0" />}
            <span className="truncate">{label}</span>
        </div>
    );
}

// ---- Sortable section header ----

function SortableSection({
    section,
    children,
    isExpanded,
    onToggleExpand,
    onRename,
    onDelete,
}: {
    section: SidebarLayoutSection;
    children: React.ReactNode;
    isExpanded: boolean;
    onToggleExpand: () => void;
    onRename?: (newTitle: string) => void;
    onDelete?: () => void;
}) {
    const {
        attributes,
        listeners,
        setNodeRef,
        transform,
        transition,
        isDragging,
    } = useSortable({ id: `section:${section.id}`, data: { type: 'section' } });

    const [editing, setEditing] = useState(false);
    const [editValue, setEditValue] = useState(section.title);

    const style = {
        transform: CSS.Translate.toString(transform),
        transition,
        opacity: isDragging ? 0 : 1,
    };

    const commitRename = () => {
        const trimmed = editValue.trim();
        if (trimmed && trimmed !== section.title) {
            onRename?.(trimmed);
        }
        setEditing(false);
    };

    return (
        <div ref={setNodeRef} style={style} className="mb-2">
            <div className="flex items-center gap-1 px-1 py-1 rounded bg-surface-light border border-border/50">
                <button
                    className="cursor-grab text-gray-500 hover:text-gray-300 shrink-0 touch-none"
                    {...attributes}
                    {...listeners}
                >
                    <Bars3Icon className="w-4 h-4" />
                </button>
                <button onClick={onToggleExpand} className="shrink-0 text-gray-400 hover:text-gray-200">
                    {isExpanded
                        ? <ChevronDownIcon className="w-3.5 h-3.5" />
                        : <ChevronRightIcon className="w-3.5 h-3.5" />
                    }
                </button>
                {editing ? (
                    <input
                        autoFocus
                        value={editValue}
                        onChange={e => setEditValue(e.target.value)}
                        onBlur={commitRename}
                        onKeyDown={e => {
                            if (e.key === 'Enter') commitRename();
                            if (e.key === 'Escape') setEditing(false);
                        }}
                        className="flex-1 min-w-0 bg-background border border-border rounded px-1.5 py-0.5 text-xs text-text outline-none focus:border-primary"
                    />
                ) : (
                    <span className="flex-1 text-xs font-semibold text-gray-300 uppercase tracking-wider truncate">
                        {section.title}
                    </span>
                )}
                <span className="text-[10px] text-gray-500 mr-1">{section.items.length}</span>
                {onRename && (
                    <button
                        onClick={() => { setEditValue(section.title); setEditing(true); }}
                        className="p-0.5 text-gray-500 hover:text-gray-300"
                        title="Rename"
                    >
                        <PencilIcon className="w-3 h-3" />
                    </button>
                )}
                {onDelete && (
                    <button
                        onClick={onDelete}
                        className="p-0.5 text-gray-500 hover:text-red-400"
                        title="Remove section"
                    >
                        <TrashIcon className="w-3 h-3" />
                    </button>
                )}
            </div>
            {isExpanded && (
                <div className="ml-2 mt-1 space-y-0.5 min-h-[28px] border-l border-border/30 pl-2">
                    {children}
                </div>
            )}
        </div>
    );
}

// ---- Draggable available item (right column) ----

function DraggableAvailableItem({ id, label, icon: Icon, onAdd }: { id: string; label: string; icon?: React.ComponentType<any>; onAdd: () => void }) {
    const {
        attributes,
        listeners,
        setNodeRef,
        transform,
        isDragging,
    } = useDraggable({ id: `avail:${id}`, data: { type: 'item', source: 'available', itemId: id } });

    const style = {
        transform: CSS.Translate.toString(transform),
        opacity: isDragging ? 0 : 1,
    };

    return (
        <div
            ref={setNodeRef}
            style={style}
            className="flex items-center gap-2 px-2 py-1.5 rounded text-sm text-gray-400 bg-surface hover:bg-white/5 group"
        >
            <button
                className="cursor-grab text-gray-500 hover:text-gray-300 shrink-0 touch-none"
                {...attributes}
                {...listeners}
            >
                <Bars3Icon className="w-3.5 h-3.5" />
            </button>
            {Icon && <Icon className="w-4 h-4 text-gray-500 shrink-0" />}
            <span className="truncate flex-1">{label}</span>
            <button
                onClick={onAdd}
                className="opacity-0 group-hover:opacity-100 p-0.5 text-primary hover:text-primary/80 transition-opacity"
                title="Add to sidebar"
            >
                <PlusIcon className="w-3.5 h-3.5" />
            </button>
        </div>
    );
}

// ---- Droppable zone for the available items column ----

function AvailableDropZone({ children }: { children: React.ReactNode }) {
    const { setNodeRef, isOver } = useDroppable({ id: 'available-bin', data: { type: 'available-bin' } });

    return (
        <div
            ref={setNodeRef}
            className={`flex-1 min-w-0 rounded transition-colors ${isOver ? 'bg-primary/5' : ''}`}
        >
            {children}
        </div>
    );
}

// ---- Item overlay (shared for drag overlay) ----

function ItemOverlay({ id }: { id: string }) {
    const def = ALL_MENU_ITEMS[id];
    if (!def) return null;
    const Icon = def.icon;
    return (
        <div className="flex items-center gap-2 px-2 py-1.5 rounded text-sm text-gray-300 bg-surface-light border border-primary/30 shadow-lg">
            {Icon && <Icon className="w-4 h-4 text-gray-400" />}
            <span>{def.label}</span>
        </div>
    );
}

// ---- Main editor component ----

interface SidebarLayoutEditorProps {
    value: SidebarLayoutSection[] | undefined;
    onChange: (value: SidebarLayoutSection[] | undefined) => void;
    isModified: boolean;
}

// Insertion preview position tracked during available→layout drags
interface InsertPreview {
    sectionId: string;
    index: number; // index in section.items where the indicator shows (-1 = end)
}

// Custom collision detection: prefer pointerWithin, fall back to rectIntersection
const customCollision: CollisionDetection = (args) => {
    const pointerCollisions = pointerWithin(args);
    if (pointerCollisions.length > 0) return pointerCollisions;
    return rectIntersection(args);
};

export default function SidebarLayoutEditor({ value, onChange }: SidebarLayoutEditorProps) {
    const { config, setConfig } = useConfig();
    const excludedItems: string[] = config?.ui?.sidebar?.excludedItems ?? [];

    const isModified = (value !== undefined && value !== null) || excludedItems.length > 0;

    const layout = useMemo((): SidebarLayoutSection[] => {
        if (value && value.length > 0) {
            const { sections, newExcluded } = reconcileLayout(value, excludedItems);
            if (newExcluded.length > 0) {
                // Persist newly discovered defaultHidden items
                setConfig('ui.sidebar.excludedItems', [...excludedItems, ...newExcluded]);
            }
            return sections;
        }
        return getDefaultLayout();
    }, [value, excludedItems]);

    const [expandedSections, setExpandedSections] = useState<Set<string>>(() =>
        new Set(layout.map((s: any) => s.id))
    );

    const [activeId, setActiveId] = useState<string | null>(null);
    const [activeType, setActiveType] = useState<'item' | 'section' | null>(null);
    const [activeSource, setActiveSource] = useState<'layout' | 'available' | null>(null);
    const [insertPreview, setInsertPreview] = useState<InsertPreview | null>(null);
    const [editingItemId, setEditingItemId] = useState<string | null>(null);
    const [editingItemValue, setEditingItemValue] = useState('');

    const sensors = useSensors(
        useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
        useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
    );

    const availableGroups = useMemo(() => {
        const groups = getAvailableGroups(layout);
        // Enrich with icon from ALL_MENU_ITEMS for rendering
        return groups.map((g: any) => ({
            ...g,
            items: g.items.map((item: any) => ({ ...item, icon: ALL_MENU_ITEMS[item.id]?.icon })),
        }));
    }, [layout]);

    const updateLayout = useCallback((newLayout: SidebarLayoutSection[]) => {
        onChange(isDefaultLayout(newLayout, excludedItems) ? undefined : newLayout);
    }, [onChange, excludedItems]);

    // ---- Helpers ----

    const findSectionForItem = (itemId: string): number => {
        return layout.findIndex((s: any) => s.items.includes(itemId));
    };

    // Resolve an overId to { sectionIdx, insertIndex }
    const resolveDropTarget = (overId: string): { sectionIdx: number; insertIndex: number } | null => {
        if (overId.startsWith('section:')) {
            const sectionIdx = layout.findIndex((s: any) => s.id === overId.replace('section:', ''));
            if (sectionIdx === -1) return null;
            return { sectionIdx, insertIndex: layout[sectionIdx].items.length };
        }
        if (overId === 'available-bin' || overId.startsWith('avail:')) return null;
        const sectionIdx = findSectionForItem(overId);
        if (sectionIdx === -1) return null;
        const insertIndex = layout[sectionIdx].items.indexOf(overId);
        return { sectionIdx, insertIndex };
    };

    // Check if a drop target is a fixed (non-modifiable) section
    const isTargetingFixed = (overId: string): boolean => {
        if (overId.startsWith('section:')) {
            return FIXED_SECTION_IDS.has(overId.replace('section:', ''));
        }
        const sectionIdx = findSectionForItem(overId);
        return sectionIdx !== -1 && FIXED_SECTION_IDS.has(layout[sectionIdx].id);
    };

    // ---- Drag handlers ----

    const handleDragStart = (event: DragStartEvent) => {
        const id = event.active.id as string;
        if (id.startsWith('section:')) {
            setActiveType('section');
            setActiveSource('layout');
            setActiveId(id.replace('section:', ''));
        } else if (id.startsWith('avail:')) {
            setActiveType('item');
            setActiveSource('available');
            setActiveId(id.replace('avail:', ''));
        } else {
            setActiveType('item');
            setActiveSource('layout');
            setActiveId(id);
        }
    };

    const handleDragOver = (event: DragOverEvent) => {
        const { active, over } = event;
        if (!over || activeType !== 'item') return;

        const overId = over.id as string;

        // Block item drops on fixed sections
        if (isTargetingFixed(overId)) {
            setInsertPreview(null);
            return;
        }

        // ---- Available item → show insertion preview ----
        if (activeSource === 'available') {
            if (overId === 'available-bin' || overId.startsWith('avail:')) {
                setInsertPreview(null);
                return;
            }
            const target = resolveDropTarget(overId);
            if (target) {
                setInsertPreview({
                    sectionId: layout[target.sectionIdx].id,
                    index: target.insertIndex,
                });
                // Auto-expand the section being hovered
                setExpandedSections(prev => {
                    const sectionId = layout[target.sectionIdx].id;
                    if (prev.has(sectionId)) return prev;
                    return new Set([...prev, sectionId]);
                });
            } else {
                setInsertPreview(null);
            }
            return;
        }

        // ---- Layout item being dragged across sections ----
        const activeItemId = active.id as string;
        const fromSectionIdx = findSectionForItem(activeItemId);
        if (fromSectionIdx === -1) return;

        if (overId === 'available-bin' || overId.startsWith('avail:')) return;

        let toSectionIdx: number;
        if (overId.startsWith('section:')) {
            toSectionIdx = layout.findIndex((s: any) => s.id === overId.replace('section:', ''));
        } else {
            toSectionIdx = findSectionForItem(overId);
        }

        if (toSectionIdx === -1 || fromSectionIdx === toSectionIdx) return;

        const newLayout = layout.map((s: any) => ({ ...s, items: [...s.items] }));
        newLayout[fromSectionIdx].items = newLayout[fromSectionIdx].items.filter((id: any) => id !== activeItemId);

        if (overId.startsWith('section:')) {
            newLayout[toSectionIdx].items.push(activeItemId);
        } else {
            const overIdx = newLayout[toSectionIdx].items.indexOf(overId);
            newLayout[toSectionIdx].items.splice(overIdx, 0, activeItemId);
        }

        updateLayout(newLayout);
    };

    const handleDragEnd = (event: DragEndEvent) => {
        const { active, over } = event;
        const dragSource = activeSource;
        const dragItemId = activeId;
        setActiveId(null);
        setActiveType(null);
        setActiveSource(null);
        setInsertPreview(null);

        if (!over || !dragItemId) return;

        const overIdStr = over.id as string;

        // Block item drops on fixed sections
        if (activeType !== 'section' && isTargetingFixed(overIdStr)) return;

        // ---- Available item dragged into layout ----
        if (dragSource === 'available') {
            if (overIdStr === 'available-bin' || overIdStr.startsWith('avail:')) return;

            const target = resolveDropTarget(overIdStr);
            if (!target) return;

            const newLayout = layout.map((s: any, i: number) => {
                if (i !== target.sectionIdx) return s;
                const newItems = [...s.items];
                newItems.splice(target.insertIndex, 0, dragItemId);
                return { ...s, items: newItems };
            });
            updateLayout(newLayout);
            if (excludedItems.includes(dragItemId)) {
                setConfig('ui.sidebar.excludedItems', excludedItems.filter(id => id !== dragItemId));
            }
            return;
        }

        // ---- Layout item dragged to available bin ----
        if (dragSource === 'layout' && activeType === 'item') {
            if (overIdStr === 'available-bin' || overIdStr.startsWith('avail:')) {
                const sectionIdx = findSectionForItem(dragItemId);
                if (sectionIdx === -1) return;
                const newLayout = layout.map((s: any, i: number) =>
                    i === sectionIdx ? { ...s, items: s.items.filter((id: any) => id !== dragItemId) } : s
                );
                updateLayout(newLayout);
                if (!excludedItems.includes(dragItemId)) {
                    setConfig('ui.sidebar.excludedItems', [...excludedItems, dragItemId]);
                }
                return;
            }
        }

        // ---- Section reorder ----
        if (activeType === 'section') {
            const fromId = (active.id as string).replace('section:', '');
            const toId = overIdStr.replace('section:', '');
            if (fromId === toId) return;
            const fromIdx = layout.findIndex((s: any) => s.id === fromId);
            const toIdx = layout.findIndex((s: any) => s.id === toId);
            if (fromIdx === -1 || toIdx === -1) return;
            updateLayout(arrayMove(layout, fromIdx, toIdx));
            return;
        }

        // ---- Item reorder within same section ----
        if (activeType === 'item' && dragSource === 'layout') {
            const activeIdStr = active.id as string;
            const sectionIdx = findSectionForItem(activeIdStr);
            if (sectionIdx === -1) return;
            if (overIdStr.startsWith('section:')) return;

            const items = layout[sectionIdx].items;
            const fromIdx = items.indexOf(activeIdStr);
            const toIdx = items.indexOf(overIdStr);
            if (fromIdx === -1 || toIdx === -1 || fromIdx === toIdx) return;

            const newLayout = layout.map((s: any, i: number) =>
                i === sectionIdx ? { ...s, items: arrayMove(items, fromIdx, toIdx) } : s
            );
            updateLayout(newLayout);
        }
    };

    const handleDragCancel = () => {
        setActiveId(null);
        setActiveType(null);
        setActiveSource(null);
        setInsertPreview(null);
    };

    // ---- Actions ----

    const removeItem = (sectionIdx: number, itemId: string) => {
        updateLayout(removeItemFromLayout(layout, sectionIdx, itemId));
        if (!excludedItems.includes(itemId)) {
            setConfig('ui.sidebar.excludedItems', [...excludedItems, itemId]);
        }
    };

    const renameItem = (sectionIdx: number, itemId: string, newLabel: string) => {
        updateLayout(renameItemInLayout(layout, sectionIdx, itemId, newLabel));
    };

    const addItemToSection = (itemId: string, sectionIdx?: number) => {
        // Default to the item's original section if no target specified
        const targetIdx = sectionIdx ?? layout.findIndex(s => s.id === ALL_MENU_ITEMS[itemId]?.defaultSection);
        updateLayout(addItemToLayout(layout, itemId, targetIdx !== -1 ? targetIdx : undefined));
        if (excludedItems.includes(itemId)) {
            setConfig('ui.sidebar.excludedItems', excludedItems.filter(id => id !== itemId));
        }
    };

    const addGroupToLayout = (sectionId: string, itemIds: string[]) => {
        const existingIdx = layout.findIndex((s: any) => s.id === sectionId);
        if (existingIdx === -1) {
            setExpandedSections(prev => new Set([...prev, sectionId]));
        }
        updateLayout(addGroupToLayoutUtil(layout, sectionId, itemIds));
        const itemSet = new Set(itemIds);
        const remaining = excludedItems.filter(id => !itemSet.has(id));
        if (remaining.length !== excludedItems.length) {
            setConfig('ui.sidebar.excludedItems', remaining);
        }
    };

    const addCustomSection = () => {
        const id = crypto.randomUUID();
        const newSection: SidebarLayoutSection = { id, title: 'New Section', items: [], isCustom: true };
        setExpandedSections(prev => new Set([...prev, id]));
        updateLayout([...layout, newSection]);
    };

    const deleteSection = (sectionIdx: number) => {
        updateLayout(deleteSectionFromLayout(layout, sectionIdx));
    };

    const renameSection = (sectionIdx: number, newTitle: string) => {
        updateLayout(renameSectionInLayout(layout, sectionIdx, newTitle));
    };

    const handleReset = () => {
        onChange(undefined);
        setConfig('ui.sidebar.excludedItems', undefined);
        setExpandedSections(new Set(getDefaultLayout().map((s: any) => s.id)));
    };

    const toggleSection = (sectionId: string) => {
        setExpandedSections(prev => {
            const next = new Set(prev);
            if (next.has(sectionId)) next.delete(sectionId);
            else next.add(sectionId);
            return next;
        });
    };

    // Build sortable IDs
    const sectionSortableIds = layout.map((s: any) => `section:${s.id}`);
    const activeDragItemId = activeId && activeType === 'item' ? activeId : null;
    const activeDragSection = activeId && activeType === 'section' ? layout.find((s: any) => s.id === activeId) : null;

    return (
        <div className="py-2">
            <div className="flex items-center justify-between mb-1">
                <div className="text-sm font-medium text-text">
                    Menu Layout
                    {isModified && <span className="ml-2 text-xs text-primary">(modified)</span>}
                </div>
                {isModified && (
                    <button
                        onClick={handleReset}
                        className="flex items-center gap-1 text-xs text-gray-400 hover:text-primary transition-colors"
                        title="Reset to defaults"
                    >
                        <ArrowUturnLeftIcon className="w-3 h-3" />
                        Reset
                    </button>
                )}
            </div>
            <p className="text-sm text-gray-300 mb-3">
                Drag to reorder sections and items. Drag between columns to show/hide.
            </p>

            <DndContext
                sensors={sensors}
                collisionDetection={customCollision}
                onDragStart={handleDragStart}
                onDragOver={handleDragOver}
                onDragEnd={handleDragEnd}
                onDragCancel={handleDragCancel}
            >
                <div className="flex gap-3" style={{ minHeight: 200 }}>
                    {/* Left column: Current layout */}
                    <div className="flex-1 min-w-0">
                        <div className="text-[11px] font-semibold text-gray-400 uppercase tracking-wider mb-2 px-1">
                            Current Layout
                        </div>
                        <SortableContext items={sectionSortableIds} strategy={verticalListSortingStrategy}>
                            {layout.map((section: any, sectionIdx: number) => {
                                const isFixed = FIXED_SECTION_IDS.has(section.id);
                                const isExpanded = expandedSections.has(section.id);
                                const itemIds = isFixed ? [] : section.items.filter((id: any) => ALL_MENU_ITEMS[id]);
                                const previewHere = !isFixed && insertPreview?.sectionId === section.id;

                                return (
                                    <SortableSection
                                        key={section.id}
                                        section={section}
                                        isExpanded={isExpanded}
                                        onToggleExpand={() => toggleSection(section.id)}
                                        onRename={isFixed ? undefined : (t) => renameSection(sectionIdx, t)}
                                        onDelete={isFixed ? undefined : () => deleteSection(sectionIdx)}
                                    >
                                        {isFixed ? (
                                            <div className="text-xs text-gray-500 italic py-2 px-2">
                                                Content auto-discovered from cluster
                                            </div>
                                        ) : (
                                            <SortableContext items={itemIds} strategy={verticalListSortingStrategy}>
                                                {itemIds.map((itemId: any, itemIdx: number) => {
                                                    const def = ALL_MENU_ITEMS[itemId];
                                                    if (!def) return null;
                                                    const displayLabel = section.itemLabels?.[itemId] || def.label;
                                                    const isEditing = editingItemId === itemId;
                                                    return (
                                                        <React.Fragment key={itemId}>
                                                            {previewHere && insertPreview!.index === itemIdx && (
                                                                <InsertionIndicator />
                                                            )}
                                                            {isEditing ? (
                                                                <div className="flex items-center gap-2 px-2 py-1.5 rounded bg-surface">
                                                                    {def.icon && <def.icon className="w-4 h-4 text-gray-400 shrink-0" />}
                                                                    <input
                                                                        autoFocus
                                                                        value={editingItemValue}
                                                                        onChange={e => setEditingItemValue(e.target.value)}
                                                                        onBlur={() => {
                                                                            renameItem(sectionIdx, itemId, editingItemValue.trim());
                                                                            setEditingItemId(null);
                                                                        }}
                                                                        onKeyDown={e => {
                                                                            if (e.key === 'Enter') {
                                                                                renameItem(sectionIdx, itemId, editingItemValue.trim());
                                                                                setEditingItemId(null);
                                                                            }
                                                                            if (e.key === 'Escape') setEditingItemId(null);
                                                                        }}
                                                                        className="flex-1 min-w-0 bg-background border border-border rounded px-1.5 py-0.5 text-xs text-text outline-none focus:border-primary"
                                                                    />
                                                                </div>
                                                            ) : (
                                                                <div className="flex items-center group">
                                                                    <div className="flex-1 min-w-0">
                                                                        <SortableItem
                                                                            id={itemId}
                                                                            label={displayLabel}
                                                                            icon={def.icon}
                                                                        />
                                                                    </div>
                                                                    <button
                                                                        onClick={() => {
                                                                            setEditingItemId(itemId);
                                                                            setEditingItemValue(displayLabel);
                                                                        }}
                                                                        className="opacity-0 group-hover:opacity-100 ml-1 p-0.5 text-gray-500 hover:text-gray-300 transition-opacity shrink-0"
                                                                        title="Rename"
                                                                    >
                                                                        <PencilIcon className="w-3 h-3" />
                                                                    </button>
                                                                    <button
                                                                        onClick={() => removeItem(sectionIdx, itemId)}
                                                                        className="opacity-0 group-hover:opacity-100 ml-0.5 p-0.5 text-gray-500 hover:text-red-400 transition-opacity shrink-0"
                                                                        title="Hide from sidebar"
                                                                    >
                                                                        <XMarkIcon className="w-3 h-3" />
                                                                    </button>
                                                                </div>
                                                            )}
                                                        </React.Fragment>
                                                    );
                                                })}
                                                {previewHere && insertPreview!.index >= itemIds.length && (
                                                    <InsertionIndicator />
                                                )}
                                                {itemIds.length === 0 && !previewHere && (
                                                    <div className="text-xs text-gray-500 italic py-2 px-2">
                                                        Drag items here
                                                    </div>
                                                )}
                                            </SortableContext>
                                        )}
                                    </SortableSection>
                                );
                            })}
                        </SortableContext>

                        <button
                            onClick={addCustomSection}
                            className="flex items-center gap-1.5 px-2 py-1.5 mt-2 text-xs text-gray-400 hover:text-primary hover:bg-white/5 rounded transition-colors w-full"
                        >
                            <PlusIcon className="w-3.5 h-3.5" />
                            Add Section
                        </button>
                    </div>

                    {/* Divider */}
                    <div className="w-px bg-border shrink-0" />

                    {/* Right column: Available (hidden) items */}
                    <AvailableDropZone>
                        <div className="text-[11px] font-semibold text-gray-400 uppercase tracking-wider mb-2 px-1">
                            Available Items
                        </div>
                        {availableGroups.length === 0 ? (
                            <div className="text-xs text-gray-500 italic px-2 py-4 text-center">
                                All items are visible
                            </div>
                        ) : (
                            <div className="space-y-3">
                                {availableGroups.map((group: any) => (
                                    <div key={group.sectionId}>
                                        <div className="flex items-center justify-between px-1 mb-1">
                                            <span className="text-[10px] font-semibold text-gray-500 uppercase tracking-wider">
                                                {group.title}
                                            </span>
                                            <button
                                                onClick={() => addGroupToLayout(group.sectionId, group.items.map((i: any) => i.id))}
                                                className="text-[10px] text-gray-500 hover:text-primary transition-colors"
                                                title={`Add all ${group.title} items`}
                                            >
                                                Add all
                                            </button>
                                        </div>
                                        <div className="space-y-0.5">
                                            {group.items.map((item: any) => (
                                                <DraggableAvailableItem
                                                    key={item.id}
                                                    id={item.id}
                                                    label={item.label}
                                                    icon={item.icon}
                                                    onAdd={() => addItemToSection(item.id)}
                                                />
                                            ))}
                                        </div>
                                    </div>
                                ))}
                            </div>
                        )}
                    </AvailableDropZone>
                </div>

                <DragOverlay dropAnimation={null}>
                    {activeDragItemId && <ItemOverlay id={activeDragItemId} />}
                    {activeDragSection && (
                        <div className="px-2 py-1 rounded bg-surface-light border border-primary/30 shadow-lg">
                            <span className="text-xs font-semibold text-gray-300 uppercase tracking-wider">
                                {activeDragSection.title}
                            </span>
                        </div>
                    )}
                </DragOverlay>
            </DndContext>
        </div>
    );
}
