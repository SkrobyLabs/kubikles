import React, { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { toRFC3339 } from './logUtils';
import { useForm, timePickerSchema } from '~/lib/validation';

/**
 * Modal for selecting a specific time to jump to in the logs.
 */
export function TimePickerModal({
    show,
    onClose,
    onApply,
    sinceTime,
    getFirstTimestamp,
    podCreationTime
}) {
    const prevShowRef = useRef(false);

    const form = useForm({
        schema: timePickerSchema,
        initialValues: { inputTime: '' },
        onSubmit: async (values) => {
            onApply(toRFC3339(values.inputTime));
        },
    });

    // Pre-fill when modal opens
    useEffect(() => {
        if (show && !prevShowRef.current) {
            if (sinceTime) {
                form.reset({ inputTime: sinceTime.replace('T', ' ').replace('Z', '') });
            } else {
                const firstTs = getFirstTimestamp();
                if (firstTs) {
                    form.reset({ inputTime: firstTs.replace('T', ' ').replace(/\.\d+Z$/, '') });
                } else if (podCreationTime) {
                    form.reset({ inputTime: podCreationTime.replace('T', ' ').replace('Z', '').slice(0, 19) });
                } else {
                    form.reset({ inputTime: '' });
                }
            }
        }
        prevShowRef.current = show;
    }, [show, sinceTime, getFirstTimestamp, podCreationTime]);

    if (!show) return null;

    return createPortal(
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
            <div className="absolute inset-0 bg-black/50" onClick={onClose} />
            <div className="relative bg-surface-light border border-border rounded-lg shadow-xl max-w-sm w-full mx-4 p-4">
                <h3 className="text-sm font-medium text-white mb-2">Jump to Time</h3>
                <p className="text-xs text-gray-400 mb-3">Show logs starting from this time (server time).</p>
                <input
                    type="text"
                    {...form.getFieldProps('inputTime')}
                    onKeyDown={(e) => {
                        if (e.key === 'Enter') form.handleSubmit();
                        if (e.key === 'Escape') onClose();
                    }}
                    placeholder="YYYY-MM-DD HH:MM:SS"
                    className="w-full px-3 py-2 text-sm bg-background border border-border rounded text-white mb-1 font-mono"
                    autoFocus
                />
                {form.touched.inputTime && form.errors.inputTime && (
                    <p className="text-xs text-red-400 mb-2">{form.errors.inputTime}</p>
                )}
                <p className="text-xs text-gray-500 mb-3">Example: 2024-11-26 14:30:00</p>
                <div className="flex justify-end gap-2">
                    <button
                        onClick={onClose}
                        className="px-3 py-1.5 text-xs text-gray-400 hover:text-white transition-colors"
                    >
                        Cancel
                    </button>
                    <button
                        onClick={form.handleSubmit}
                        disabled={!form.isValid}
                        className="px-3 py-1.5 text-xs bg-primary text-white rounded hover:bg-primary/80 transition-colors disabled:opacity-50"
                    >
                        Apply
                    </button>
                </div>
            </div>
        </div>,
        document.body
    );
}
