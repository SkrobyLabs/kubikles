import React, { memo } from 'react';

/**
 * Horizontal bar showing resource usage percentage
 * @param {number} percent - Usage percentage (0-100)
 * @param {string} label - Short label (e.g., "CPU", "Mem")
 * @param {string} tooltipLabel - Label for tooltip (e.g., "Used", "Reserved")
 * @param {string} color - Tailwind color class (e.g., "bg-blue-500")
 * @param {boolean} fixedColor - If true, don't override color based on thresholds
 */
const ResourceBar = memo(function ResourceBar({ percent, label, tooltipLabel, color = 'bg-blue-500', fixedColor = false }) {
    // Clamp percent to 0-100
    const clampedPercent = Math.max(0, Math.min(100, percent));

    // Color coding based on usage level (unless fixedColor is true)
    let barColor = color;
    if (!fixedColor) {
        if (clampedPercent >= 90) {
            barColor = 'bg-red-500';
        } else if (clampedPercent >= 70) {
            barColor = 'bg-yellow-500';
        }
    }

    const titleText = tooltipLabel || label;

    return (
        <div
            className="flex items-center gap-1"
            title={titleText ? `${titleText}: ${clampedPercent}%` : `${clampedPercent}%`}
        >
            {label && <span className="text-[10px] text-gray-500 w-7">{label}</span>}
            <div className="w-12 h-2 bg-gray-700 rounded-sm overflow-hidden">
                <div
                    className={`h-full ${barColor} transition-all duration-300`}
                    style={{ width: `${clampedPercent}%` }}
                />
            </div>
            <span className="text-[10px] text-gray-400 w-8 text-right">{clampedPercent}%</span>
        </div>
    );
});

export default ResourceBar;
