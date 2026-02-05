import { useState, useEffect } from 'react';
import { GetCRDPrinterColumns } from '../../wailsjs/go/main/App';

export interface CRDPrinterColumn {
    name: string;
    type: string;
    description?: string;
    jsonPath: string;
    format?: string;
    priority?: number;
}

interface UseCRDPrinterColumnsResult {
    columns: CRDPrinterColumn[];
    loading: boolean;
    error: Error | null;
}

/**
 * Hook to fetch additional printer columns for a CRD.
 */
export const useCRDPrinterColumns = (
    group: string,
    resource: string,
    isVisible: boolean
): UseCRDPrinterColumnsResult => {
    const [columns, setColumns] = useState<CRDPrinterColumn[]>([]);
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<Error | null>(null);

    useEffect(() => {
        if (!group || !resource || !isVisible) {
            return;
        }

        const fetchColumns = async (): Promise<void> => {
            setLoading(true);
            try {
                // CRD name is typically resource.group (e.g., certificates.cert-manager.io)
                const crdName = `${resource}.${group}`;
                const cols = await GetCRDPrinterColumns(crdName);
                setColumns(cols || []);
                setError(null);
            } catch (err) {
                console.error("Failed to fetch CRD printer columns", err);
                setError(err as Error);
                setColumns([]);
            } finally {
                setLoading(false);
            }
        };

        fetchColumns();
    }, [group, resource, isVisible]);

    return { columns, loading, error };
};
