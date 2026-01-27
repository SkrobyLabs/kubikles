import React, { createContext, useContext, useState, useMemo, useCallback, useEffect } from 'react';

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

    // Auto-close menus when the window loses focus (e.g. tab switch, alt-tab)
    useEffect(() => {
        const handleBlur = () => setActiveMenuId(null);
        window.addEventListener('blur', handleBlur);
        return () => window.removeEventListener('blur', handleBlur);
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
