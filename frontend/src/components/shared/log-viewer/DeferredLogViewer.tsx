import React, { useState, useEffect, useRef } from 'react';
import LogViewer from './LogViewer';
import { ExclamationTriangleIcon } from '@heroicons/react/24/outline';

export interface ResolvedLogViewerProps {
    namespace: any;
    pod: any;
    containers?: any;
    siblingPods?: any;
    podContainerMap?: any;
    ownerName?: any;
    podCreationTime?: any;
}

interface DeferredLogViewerProps {
    resolve: () => Promise<ResolvedLogViewerProps | null>;
    tabContext?: string;
}

export default function DeferredLogViewer({ resolve, tabContext }: DeferredLogViewerProps) {
    const [state, setState] = useState<'loading' | 'resolved' | 'empty' | 'error'>('loading');
    const [props, setProps] = useState<ResolvedLogViewerProps | null>(null);
    const [error, setError] = useState('');
    const mountedRef = useRef(true);

    useEffect(() => {
        mountedRef.current = true;
        let cancelled = false;

        resolve()
            .then((result) => {
                if (cancelled || !mountedRef.current) return;
                if (result) {
                    setProps(result);
                    setState('resolved');
                } else {
                    setState('empty');
                }
            })
            .catch((err) => {
                if (cancelled || !mountedRef.current) return;
                setError(String(err.message || err));
                setState('error');
            });

        return () => {
            cancelled = true;
            mountedRef.current = false;
        };
    }, []);

    if (state === 'loading') {
        return (
            <div className="flex flex-col items-center justify-center h-full bg-background">
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
                <span className="mt-3 text-gray-500 text-sm">Resolving pods...</span>
            </div>
        );
    }

    if (state === 'empty') {
        return (
            <div className="flex flex-col items-center justify-center h-full bg-background text-gray-400">
                <ExclamationTriangleIcon className="h-8 w-8 mb-2 text-amber-500" />
                <span>No pods found for this resource.</span>
            </div>
        );
    }

    if (state === 'error') {
        return (
            <div className="flex flex-col items-center justify-center h-full bg-background text-gray-400">
                <ExclamationTriangleIcon className="h-8 w-8 mb-2 text-red-500" />
                <span className="text-red-400">Failed to resolve pods</span>
                <span className="text-sm mt-1 text-gray-500 max-w-md text-center">{error}</span>
            </div>
        );
    }

    return (
        <LogViewer
            namespace={props!.namespace}
            pod={props!.pod}
            containers={props!.containers}
            siblingPods={props!.siblingPods}
            podContainerMap={props!.podContainerMap}
            ownerName={props!.ownerName}
            podCreationTime={props!.podCreationTime}
            tabContext={tabContext}
        />
    );
}
