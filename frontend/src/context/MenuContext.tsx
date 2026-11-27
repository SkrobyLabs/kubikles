import React, { createContext, useContext, useEffect, useState, useSyncExternalStore } from 'react';

export type MenuPosition = {
    top?: number;
    bottom?: number;
    left: number;
    maxHeight: number;
};

const DEFAULT_MENU_WIDTH = 192;
const DEFAULT_MENU_HEIGHT = 360;

interface MenuState {
    activeMenuId: string | null;
    menuPosition: MenuPosition;
}

type SetActiveMenuId = React.Dispatch<React.SetStateAction<string | null>>;

/**
 * Only one row menu is ever open, so the open id and its placement live in one tiny
 * external store. Row cells subscribe through selectors (useMenuOpen), which lets opening
 * a menu re-render exactly the two affected cells instead of the whole list and its
 * column definitions.
 */
export interface MenuStore {
    getState(): MenuState;
    subscribe(listener: () => void): () => void;
    setActiveMenuId: SetActiveMenuId;
    changeMenu(isOpen: boolean, menuId: string, buttonElement: HTMLElement | null, menuWidth?: number, estimatedMenuHeight?: number): void;
}

export function createMenuStore(): MenuStore {
    let state: MenuState = { activeMenuId: null, menuPosition: { top: 0, left: 0, maxHeight: DEFAULT_MENU_HEIGHT } };
    const listeners = new Set<() => void>();
    const update = (next: MenuState) => {
        if (next.activeMenuId === state.activeMenuId && next.menuPosition === state.menuPosition) return;
        state = next;
        listeners.forEach(l => l());
    };

    const setActiveMenuId: SetActiveMenuId = (value) => {
        const id = typeof value === 'function' ? value(state.activeMenuId) : value;
        update({ ...state, activeMenuId: id });
    };

    const changeMenu: MenuStore['changeMenu'] = (isOpen, menuId, buttonElement, menuWidth = DEFAULT_MENU_WIDTH, estimatedMenuHeight = DEFAULT_MENU_HEIGHT) => {
        let menuPosition = state.menuPosition;
        if (isOpen && buttonElement) {
            const rect = buttonElement.getBoundingClientRect();
            const viewportWidth = window.innerWidth;
            const viewportHeight = window.innerHeight;
            const margin = 8;
            const gap = 4;

            let left = rect.right - menuWidth;
            if (left < margin) left = margin;
            // Flip to the left of the button when the menu would leave the viewport on the right
            if (left + menuWidth > viewportWidth - margin) {
                left = rect.left - menuWidth;
                if (left < margin) left = viewportWidth - menuWidth - margin;
            }

            const availableBelow = Math.max(0, viewportHeight - rect.bottom - gap - margin);
            const availableAbove = Math.max(0, rect.top - gap - margin);
            const opensUp = availableBelow < estimatedMenuHeight && availableAbove > availableBelow;
            const viewportMaxHeight = Math.max(48, viewportHeight - (margin * 2));
            const maxHeight = Math.max(48, Math.min(Math.floor(opensUp ? availableAbove : availableBelow), viewportMaxHeight));

            menuPosition = opensUp
                ? { bottom: Math.max(margin, viewportHeight - rect.top + gap), left, maxHeight }
                : { top: Math.max(margin, Math.min(rect.bottom + gap, viewportHeight - margin - maxHeight)), left, maxHeight };
        }
        update({ activeMenuId: isOpen ? menuId : null, menuPosition });
    };

    return {
        getState: () => state,
        subscribe: (listener) => { listeners.add(listener); return () => { listeners.delete(listener); }; },
        setActiveMenuId,
        changeMenu,
    };
}

const MenuContext = createContext<MenuStore | undefined>(undefined);

export const useMenuStore = (): MenuStore => {
    const store = useContext(MenuContext);
    if (!store) {
        throw new Error('useMenu must be used within a MenuProvider');
    }
    return store;
};

/**
 * Legacy accessor: re-renders its caller whenever the open menu changes. Prefer
 * useMenuOpen / useMenuActions inside row cells so lists do not re-render.
 */
export const useMenu = (): { activeMenuId: string | null; setActiveMenuId: SetActiveMenuId } => {
    const store = useMenuStore();
    const activeMenuId = useSyncExternalStore(store.subscribe, () => store.getState().activeMenuId, () => null);
    return { activeMenuId, setActiveMenuId: store.setActiveMenuId };
};

/** True while the given menu is the open one; re-renders only when that answer flips. */
export const useMenuOpen = (menuId: string): boolean => {
    const store = useMenuStore();
    return useSyncExternalStore(store.subscribe, () => store.getState().activeMenuId === menuId, () => false);
};

/** Placement of the open menu. Only meaningful while a menu is open. */
export const useMenuPlacement = (enabled: boolean): MenuPosition => {
    const store = useMenuStore();
    // Closed cells keep a constant snapshot so a placement change does not re-render them.
    const read = () => (enabled ? store.getState().menuPosition : CLOSED_POSITION);
    return useSyncExternalStore(store.subscribe, read, () => CLOSED_POSITION);
};

const CLOSED_POSITION: MenuPosition = { top: 0, left: 0, maxHeight: DEFAULT_MENU_HEIGHT };

/** Stable store handle for event handlers; never causes a re-render. */
export const useMenuActions = (): MenuStore => useMenuStore();

/**
 * MenuProvider keeps the volatile open-menu state separate from UIContext. The context
 * value is a stable store, so the provider itself never re-renders its subtree.
 */
export const MenuProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [store] = useState(createMenuStore);

    // Auto-close menus when the window loses focus (e.g. tab switch, alt-tab)
    useEffect(() => {
        const handleBlur = (): void => store.setActiveMenuId(null);
        window.addEventListener('blur', handleBlur);
        return () => window.removeEventListener('blur', handleBlur);
    }, [store]);

    return (
        <MenuContext.Provider value={store}>
            {children}
        </MenuContext.Provider>
    );
};
