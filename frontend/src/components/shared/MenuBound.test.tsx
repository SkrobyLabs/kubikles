import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { MenuProvider } from '~/context/MenuContext';
import MenuBound from './MenuBound';

const Probe = ({ isOpen, menuPosition, onOpenChange, label }: any) => (
    <span data-open={String(isOpen)} data-left={menuPosition.left} data-fn={typeof onOpenChange}>{label}</span>
);

describe('MenuBound', () => {
    it('injects open state, placement and a change handler into the wrapped menu', () => {
        const html = renderToStaticMarkup(
            <MenuProvider><MenuBound as={Probe} menuId="pod-1" label="menu" /></MenuProvider>,
        );
        expect(html).toContain('data-open="false"');
        expect(html).toContain('data-fn="function"');
        expect(html).toContain('>menu<');
    });
});
