import React from 'react';

/**
 * Simple wrapper component for details panels.
 * Renders children directly with overflow handling.
 */
export default function DetailsPanel({ children }: any) {
    return (
        <div className="h-full overflow-auto">
            {children}
        </div>
    );
}
