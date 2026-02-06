import React, { useState, useRef, useEffect } from 'react';
import { createPortal } from 'react-dom';

export default function Tooltip({ children, content, position = 'bottom' }: { children: React.ReactNode; content: React.ReactNode; position?: 'top' | 'bottom' }) {
    const [isVisible, setIsVisible] = useState(false);
    const [coords, setCoords] = useState({ top: 0, left: 0 });
    const triggerRef = useRef<HTMLSpanElement>(null);
    const tooltipRef = useRef<HTMLDivElement>(null);
    const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

    const showTooltip = () => {
        timeoutRef.current = setTimeout(() => {
            setIsVisible(true);
        }, 300);
    };

    const hideTooltip = () => {
        if (timeoutRef.current) {
            clearTimeout(timeoutRef.current);
        }
        setIsVisible(false);
    };

    useEffect(() => {
        if (isVisible && triggerRef.current) {
            const rect = triggerRef.current.getBoundingClientRect();
            const tooltipEl = tooltipRef.current;

            let top, left;

            // Calculate position based on preference
            if (position === 'top') {
                top = rect.top - 8;
                left = rect.left + rect.width / 2;
            } else {
                // Default to bottom
                top = rect.bottom + 8;
                left = rect.left + rect.width / 2;
            }

            // Adjust if tooltip would go off-screen
            if (tooltipEl) {
                const tooltipRect = tooltipEl.getBoundingClientRect();

                // Check right edge
                if (left + tooltipRect.width / 2 > window.innerWidth - 10) {
                    left = window.innerWidth - tooltipRect.width / 2 - 10;
                }

                // Check left edge
                if (left - tooltipRect.width / 2 < 10) {
                    left = tooltipRect.width / 2 + 10;
                }

                // Check bottom edge - flip to top if needed
                if (position === 'bottom' && top + tooltipRect.height > window.innerHeight - 10) {
                    top = rect.top - tooltipRect.height - 8;
                }

                // Check top edge - flip to bottom if needed
                if (position === 'top' && top - tooltipRect.height < 10) {
                    top = rect.bottom + 8;
                }
            }

            setCoords({ top, left });
        }
    }, [isVisible, position]);

    useEffect(() => {
        return () => {
            if (timeoutRef.current) {
                clearTimeout(timeoutRef.current);
            }
        };
    }, []);

    if (!content) return children;

    return (
        <>
            <span
                ref={triggerRef}
                onMouseEnter={showTooltip}
                onMouseLeave={hideTooltip}
                className="inline-flex"
            >
                {children}
            </span>
            {isVisible && createPortal(
                <div
                    ref={tooltipRef}
                    className="fixed z-[9999] px-2 py-1 text-xs bg-background border border-border text-white rounded shadow-lg whitespace-nowrap pointer-events-none transform -translate-x-1/2"
                    style={{
                        top: `${coords.top}px`,
                        left: `${coords.left}px`,
                    }}
                >
                    {content}
                </div>,
                document.body
            )}
        </>
    );
}
