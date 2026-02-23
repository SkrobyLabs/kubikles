import React, { createContext, useContext, useState, useMemo, useEffect } from 'react';

// Menu context value interface
interface MenuContextValue {
    activeMenuId: string | null;
    setActiveMenuId: React.Dispatch<React.SetStateAction<string | null>>;
}

const MenuContext = createContext<MenuContextValue | undefined>(undefined);

export const useMenu = (): MenuContextValue => {
    const context = useContext(MenuContext);
    if (!context) {
        throw new Error('useMenu must be used within a MenuProvider');
    }
    return context;
};

/**
 * MenuProvider handles the volatile activeMenuId state separately from UIContext.
 * This prevents all UIContext consumers from re-rendering when menus open/close.
 */
export const MenuProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [activeMenuId, setActiveMenuId] = useState<string | null>(null);

    // Auto-close menus when the window loses focus (e.g. tab switch, alt-tab)
    useEffect(() => {
        const handleBlur = (): void => setActiveMenuId(null);
        window.addEventListener('blur', handleBlur);
        return () => window.removeEventListener('blur', handleBlur);
    }, []);

    const value: MenuContextValue = useMemo(() => ({
        activeMenuId,
        setActiveMenuId,
    }), [activeMenuId]);

    return (
        <MenuContext.Provider value={value}>
            {children}
        </MenuContext.Provider>
    );
};
