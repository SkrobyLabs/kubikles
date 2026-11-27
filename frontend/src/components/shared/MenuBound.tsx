import React, { useCallback } from 'react';
import { useMenuActions, useMenuOpen, useMenuPlacement } from '~/context/MenuContext';

/**
 * Binds a row actions menu to the shared open-menu store. Because the cell itself
 * subscribes, list column definitions never depend on which menu is open, and opening a
 * menu re-renders only the cell that closes and the cell that opens.
 */
function MenuBoundImpl({ as: Menu, menuId, ...rest }: { as: React.ComponentType<any>; menuId: string; [prop: string]: any }) {
    const isOpen = useMenuOpen(menuId);
    const menuPosition = useMenuPlacement(isOpen);
    const store = useMenuActions();
    const onOpenChange = useCallback(
        (open: boolean, buttonElement?: HTMLElement | null) => store.changeMenu(open, menuId, buttonElement ?? null),
        [store, menuId],
    );
    return <Menu {...rest} isOpen={isOpen} menuPosition={menuPosition} onOpenChange={onOpenChange} />;
}

const MenuBound = React.memo(MenuBoundImpl);
export default MenuBound;
