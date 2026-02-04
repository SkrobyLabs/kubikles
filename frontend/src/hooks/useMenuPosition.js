import { useState, useCallback } from 'react';
import { useMenu } from '../context';

/**
 * Hook for managing dropdown menu positioning with edge detection.
 * Extracts common menu positioning logic used across all List components.
 *
 * @param {number} menuWidth - Width of the dropdown menu in pixels (default: 192)
 * @returns {Object} - { activeMenuId, menuPosition, handleMenuOpenChange }
 */
export function useMenuPosition(menuWidth = 192) {
    const { activeMenuId, setActiveMenuId } = useMenu();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });

    const handleMenuOpenChange = useCallback((isOpen, menuId, buttonElement) => {
        if (isOpen && buttonElement) {
            const rect = buttonElement.getBoundingClientRect();
            const viewportWidth = window.innerWidth;

            // Calculate position with edge detection
            let left = rect.right - menuWidth;

            // Don't go off left edge
            if (left < 8) {
                left = 8;
            }

            // If menu would go off right edge, flip to left side of button
            if (left + menuWidth > viewportWidth - 8) {
                left = rect.left - menuWidth;
                // If still off-screen, just pin to right edge
                if (left < 8) {
                    left = viewportWidth - menuWidth - 8;
                }
            }

            setMenuPosition({
                top: rect.bottom + 4,
                left
            });
        }
        setActiveMenuId(isOpen ? menuId : null);
    }, [setActiveMenuId, menuWidth]);

    return { activeMenuId, menuPosition, handleMenuOpenChange };
}
