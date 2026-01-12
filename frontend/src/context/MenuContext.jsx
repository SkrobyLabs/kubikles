import React, { createContext, useContext, useState, useMemo, useCallback } from 'react';

const MenuContext = createContext();

export const useMenu = () => {
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
export const MenuProvider = ({ children }) => {
    const [activeMenuId, setActiveMenuId] = useState(null);

    const closeMenu = useCallback(() => {
        setActiveMenuId(null);
    }, []);

    const value = useMemo(() => ({
        activeMenuId,
        setActiveMenuId,
        closeMenu
    }), [activeMenuId, closeMenu]);

    return (
        <MenuContext.Provider value={value}>
            {children}
        </MenuContext.Provider>
    );
};
