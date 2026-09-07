import React from 'react';
import type { ResourceLoadState } from '~/hooks/useResource';
import { formatCount } from './resourceListColumns';

export default function ResourceLoadingStatus({ state, displayed }: { state?: ResourceLoadState; displayed: number }) {
    if (!state || state.phase === 'complete') return null;
    return (
        <div role="status" aria-live="polite" className="pointer-events-none absolute bottom-3 left-3 right-3 z-20 mx-auto w-fit max-w-[calc(100%_-_1.5rem)] rounded-lg border border-border bg-surface/95 px-4 py-2 text-sm text-gray-400 shadow-lg backdrop-blur-sm">
            {state.phase === 'incomplete' ? <>
                Loading failed. Displayed resources may be incomplete or outdated. {state.error?.message}
                <button className="pointer-events-auto ml-3 text-primary underline" onClick={state.retry}>Retry</button>
            </> : state.phase === 'refreshing' ? 'Refreshing resources… Displayed resources may change.' : 'Loading resources… More resources may arrive.'}
            {state.phase !== 'incomplete' && state.progress && <span className="ml-2">
                {formatCount(state.progress.loaded)} fetched{state.progress.total > 0 ? ` of ${formatCount(state.progress.total)}` : ''}.
            </span>}
            <span className="ml-2">{formatCount(displayed)} displayed. Counts and select-all apply to displayed resources.</span>
        </div>
    );
}
