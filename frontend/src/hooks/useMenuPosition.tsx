import { useCallback, useSyncExternalStore } from 'react';
import { useMenuStore } from '../context/MenuContext';

/**
 * Hook for managing dropdown menu positioning with edge detection.
 *
 * The open id and placement live in the shared menu store (one menu is open at a time), so
 * `handleMenuOpenChange` is referentially stable. List components should not read
 * `activeMenuId` while building column definitions: use <MenuBound> in the actions cell so
 * opening a menu does not rebuild columns and re-sort the table.
 *
 * @param {number} menuWidth - Width of the dropdown menu in pixels (default: 192)
 * @param {number} estimatedMenuHeight - Expected menu height in pixels for vertical placement (default: 360)
 * @returns {Object} - { activeMenuId, menuPosition, handleMenuOpenChange }
 */
export function useMenuPosition(menuWidth = 192, estimatedMenuHeight = 360) {
    const store = useMenuStore();
    const activeMenuId = useSyncExternalStore(store.subscribe, () => store.getState().activeMenuId);
    const menuPosition = useSyncExternalStore(store.subscribe, () => store.getState().menuPosition);
    const handleMenuOpenChange = useCallback((isOpen: boolean, menuId: string, buttonElement: HTMLElement | null) =>
        store.changeMenu(isOpen, menuId, buttonElement, menuWidth, estimatedMenuHeight), [store, menuWidth, estimatedMenuHeight]);
    return { activeMenuId, menuPosition, handleMenuOpenChange };
}
