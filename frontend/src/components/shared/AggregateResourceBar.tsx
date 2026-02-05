import React, { memo } from 'react';

// Tooltip text for over-committed resources
const OVERCOMMIT_TOOLTIP = "Over-committed: Some containers use more than their requests. Works because other pods aren't using full reservations.";

/**
 * Aggregate bar showing usage, reserved (optional), and committed as stacked segments
 *
 * @param {number} usagePercent - Actual usage percentage
 * @param {number} reservedPercent - Reserved/requested percentage (optional, for nodes)
 * @param {number} committedPercent - Committed percentage (max of usage, reserved per container)
 * @param {string} type - "cpu" or "memory" (currently unused, kept for future use)
 * @param {string} label - Optional label prefix for tooltip
 * @param {boolean} showPercent - Whether to show percentage text next to bar (default: true)
 * @param {string} barClassName - Custom class for bar width/height (default: "w-16 h-3")
 * @param {number} usageValue - Raw usage value for tooltip display
 * @param {number} reservedValue - Raw reserved value for tooltip display
 * @param {number} committedValue - Raw committed value for tooltip display
 * @param {number} capacityValue - Raw capacity value for tooltip display
 * @param {function} formatValue - Function to format raw values (e.g., formatCpu, formatBytes)
 */
const AggregateResourceBar = memo(function AggregateResourceBar({
    usagePercent = 0,
    reservedPercent,
    committedPercent,
    type = 'cpu',
    label = '',
    showPercent = true,
    barClassName = 'w-16 h-3',
    usageValue,
    reservedValue,
    committedValue,
    capacityValue,
    formatValue
}) {
    // Clamp values
    const usage = Math.max(0, Math.min(100, usagePercent || 0));
    const reserved = reservedPercent != null ? Math.max(0, Math.min(100, reservedPercent)) : null;
    const committed = committedPercent != null ? Math.max(0, committedPercent) : null; // Don't clamp committed to allow >100%

    // Calculate reserved excess (portion of reserved beyond usage)
    const reservedExcess = reserved != null ? Math.max(0, reserved - usage) : 0;

    // Calculate committed excess (portion of committed beyond usage and reserved)
    const baseForCommitted = reserved != null ? Math.max(usage, reserved) : usage;
    const committedExcess = committed != null ? Math.max(0, Math.min(100, committed) - baseForCommitted) : 0;

    // Check if over-committed
    const isOverCommitted = committed != null && committed > 100;

    // Helper to format value if formatValue function is provided
    const fmtVal = (val) => formatValue && val != null ? ` (${formatValue(val)})` : '';

    // Build tooltip lines
    const tooltipLines = [];
    if (label) tooltipLines.push(label);
    tooltipLines.push(`Usage: ${Math.round(usage)}%${fmtVal(usageValue)}`);
    if (reserved != null) tooltipLines.push(`Reserved: ${Math.round(reserved)}%${fmtVal(reservedValue)}`);
    if (committed != null) {
        tooltipLines.push(`Committed: ${Math.round(committed)}%${fmtVal(committedValue)}${isOverCommitted ? ' *' : ''}`);
    }
    if (capacityValue != null && formatValue) {
        tooltipLines.push(`Capacity: ${formatValue(capacityValue)}`);
    }
    if (isOverCommitted) {
        tooltipLines.push('');
        tooltipLines.push(OVERCOMMIT_TOOLTIP);
    }

    const tooltip = tooltipLines.join('\n');

    // Always use blue for usage (consistent across CPU and memory)
    const usageColor = 'bg-blue-500';

    // Display percentage (show committed if available, otherwise usage)
    const displayPercent = committed != null ? Math.round(committed) : Math.round(usage);

    return (
        <div className="flex items-center gap-1.5" title={tooltip}>
            <div className={`${barClassName} bg-gray-700 rounded-sm overflow-hidden relative flex`}>
                {/* Usage (blue) */}
                <div
                    className={`h-full ${usageColor} shrink-0`}
                    style={{ width: `${usage}%` }}
                />
                {/* Reserved excess (yellow) - only shows if reserved > usage */}
                {reservedExcess > 0 && (
                    <div
                        className="h-full bg-yellow-500 shrink-0"
                        style={{ width: `${Math.min(100 - usage, reservedExcess)}%` }}
                    />
                )}
                {/* Committed excess (red) - shows committed beyond usage and reserved */}
                {committedExcess > 0 && (
                    <div
                        className="h-full bg-red-500 shrink-0"
                        style={{ width: `${committedExcess}%` }}
                    />
                )}
            </div>
            {showPercent && (
                <span className={`text-[10px] w-8 text-right ${isOverCommitted ? 'text-yellow-400' : 'text-gray-400'}`}>
                    {displayPercent}%{isOverCommitted && '*'}
                </span>
            )}
        </div>
    );
});

export default AggregateResourceBar;
