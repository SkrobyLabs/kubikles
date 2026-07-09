import { useState, useCallback } from 'react';
import { useMenu } from '../context';

type MenuPosition = {
    top?: number;
    bottom?: number;
    left: number;
    maxHeight: number;
};

/**
 * Hook for managing dropdown menu positioning with edge detection.
 * Extracts common menu positioning logic used across all List components.
 *
 * @param {number} menuWidth - Width of the dropdown menu in pixels (default: 192)
 * @param {number} estimatedMenuHeight - Expected menu height in pixels for vertical placement (default: 360)
 * @returns {Object} - { activeMenuId, menuPosition, handleMenuOpenChange }
 */
export function useMenuPosition(menuWidth = 192, estimatedMenuHeight = 360) {
    const { activeMenuId, setActiveMenuId } = useMenu();
    const [menuPosition, setMenuPosition] = useState<MenuPosition>({ top: 0, left: 0, maxHeight: estimatedMenuHeight });

    const handleMenuOpenChange = useCallback((isOpen: boolean, menuId: string, buttonElement: HTMLElement | null) => {
        if (isOpen && buttonElement) {
            const rect = buttonElement.getBoundingClientRect();
            const viewportWidth = window.innerWidth;
            const viewportHeight = window.innerHeight;
            const margin = 8;
            const gap = 4;

            // Calculate position with edge detection
            let left = rect.right - menuWidth;

            // Don't go off left edge
            if (left < margin) {
                left = margin;
            }

            // If menu would go off right edge, flip to left side of button
            if (left + menuWidth > viewportWidth - margin) {
                left = rect.left - menuWidth;
                // If still off-screen, just pin to right edge
                if (left < margin) {
                    left = viewportWidth - menuWidth - margin;
                }
            }

            const availableBelow = Math.max(0, viewportHeight - rect.bottom - gap - margin);
            const availableAbove = Math.max(0, rect.top - gap - margin);
            const opensUp = availableBelow < estimatedMenuHeight && availableAbove > availableBelow;
            const viewportMaxHeight = Math.max(48, viewportHeight - (margin * 2));
            const maxHeight = Math.max(48, Math.min(Math.floor(opensUp ? availableAbove : availableBelow), viewportMaxHeight));

            setMenuPosition(opensUp
                ? {
                    bottom: Math.max(margin, viewportHeight - rect.top + gap),
                    left,
                    maxHeight
                }
                : {
                    top: Math.max(margin, Math.min(rect.bottom + gap, viewportHeight - margin - maxHeight)),
                    left,
                    maxHeight
                });
        }
        setActiveMenuId(isOpen ? menuId : null);
    }, [setActiveMenuId, menuWidth, estimatedMenuHeight]);

    return { activeMenuId, menuPosition, handleMenuOpenChange };
}
