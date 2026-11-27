import React from 'react';
import { CheckCircleIcon, XCircleIcon } from '@heroicons/react/24/outline';
import { formatAge } from '~/utils/formatting';

/**
 * Renders a value based on its type from the CRD column definition. Shared by the legacy list
 * (values evaluated in the browser) and the windowed list (values computed by the backend).
 */
export const renderColumnValue = (value: any, type: any): React.ReactNode => {
    if (value === undefined || value === null) return '-';

    switch (type) {
        case 'date':
            return formatAge(value);
        case 'boolean':
            // Show a colored indicator for boolean values
            if (value === true || value === 'True' || value === 'true') {
                return <CheckCircleIcon className="h-5 w-5 text-green-400" />;
            } else if (value === false || value === 'False' || value === 'false') {
                return <XCircleIcon className="h-5 w-5 text-red-400" />;
            }
            return String(value);
        case 'integer':
        case 'number':
            return String(value);
        default:
            // For status-like values, add color coding
            if (typeof value === 'string') {
                if (value === 'True' || value === 'Ready' || value === 'Active' || value === 'Bound' || value === 'Running') {
                    return <span className="text-green-400">{value}</span>;
                } else if (value === 'False' || value === 'Failed' || value === 'Error') {
                    return <span className="text-red-400">{value}</span>;
                } else if (value === 'Pending' || value === 'Unknown' || value === 'Progressing') {
                    return <span className="text-yellow-400">{value}</span>;
                }
            }
            return String(value);
    }
};
