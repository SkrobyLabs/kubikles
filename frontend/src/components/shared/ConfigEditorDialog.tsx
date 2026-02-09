import React, { useEffect } from 'react';
import { createPortal } from 'react-dom';
import { useConfig } from '~/context';
import ConfigEditor from './ConfigEditor';

export default function ConfigEditorDialog() {
    const { showConfigEditor, closeConfigEditor } = useConfig();

    useEffect(() => {
        if (!showConfigEditor) return;
        const handleKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') {
                e.stopPropagation();
                closeConfigEditor();
            }
        };
        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [showConfigEditor, closeConfigEditor]);

    if (!showConfigEditor) return null;

    return createPortal(
        <div className="fixed inset-0 z-[90] flex items-center justify-center">
            <div className="absolute inset-0 bg-black/50" onClick={closeConfigEditor} />
            <div className="relative bg-surface border border-border rounded-lg shadow-xl w-[1400px] max-w-[90vw] h-[80vh] flex flex-col overflow-hidden">
                <ConfigEditor />
            </div>
        </div>,
        document.body
    );
}
