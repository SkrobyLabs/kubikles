import { useState, useEffect } from 'react';
import { GetCRDPrinterColumns } from '../../wailsjs/go/main/App';

/**
 * Hook to fetch additional printer columns for a CRD.
 * @param {string} group - The API group (e.g., 'cert-manager.io')
 * @param {string} resource - The plural resource name (e.g., 'certificates')
 * @param {boolean} isVisible - Whether the component is visible
 * @returns {{ columns: Array, loading: boolean, error: Error|null }}
 */
export const useCRDPrinterColumns = (group, resource, isVisible) => {
    const [columns, setColumns] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    useEffect(() => {
        if (!group || !resource || !isVisible) {
            return;
        }

        const fetchColumns = async () => {
            setLoading(true);
            try {
                // CRD name is typically resource.group (e.g., certificates.cert-manager.io)
                const crdName = `${resource}.${group}`;
                const cols = await GetCRDPrinterColumns(crdName);
                setColumns(cols || []);
                setError(null);
            } catch (err) {
                console.error("Failed to fetch CRD printer columns", err);
                setError(err);
                setColumns([]);
            } finally {
                setLoading(false);
            }
        };

        fetchColumns();
    }, [group, resource, isVisible]);

    return { columns, loading, error };
};
