import React, { useEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { ExecuteResourceAction, PrepareResourceAction } from 'wailsjs/go/main/App';
import { useK8s } from '~/context';
import type { ActionPlan, ActionResult, OperatorAction, ResourceRef } from './types';

export default function OperatorActionDialog({ action, source, onClose }: {
    action: OperatorAction; source: ResourceRef; onClose: () => void;
}) {
    const { currentContext } = useK8s();
    const [mode, setMode] = useState(action.modes[0].id);
    const [plan, setPlan] = useState<ActionPlan | null>(null);
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [loading, setLoading] = useState(true);
    const [running, setRunning] = useState(false);
    const [error, setError] = useState('');
    const [results, setResults] = useState<ActionResult[] | null>(null);
    const [revision, setRevision] = useState(0);
    const executing = useRef(false);
    const dialogRef = useRef<HTMLDivElement>(null);
    const stale = source.context !== currentContext;
    const selectable = action.modes.find(item => item.id === mode)?.selectTargets;
    useEffect(() => {
        let cancelled = false;
        setPlan(null); setSelected(new Set()); setError(''); setLoading(true);
        PrepareResourceAction(source, action.id, mode).then((next: ActionPlan) => {
            if (!cancelled) setPlan(next);
        }).catch((err: unknown) => { if (!cancelled) setError(String(err)); })
            .finally(() => { if (!cancelled) setLoading(false); });
        return () => { cancelled = true; };
    }, [source, action.id, mode, revision]);
    useEffect(() => {
        const previousFocus = document.activeElement as HTMLElement | null;
        dialogRef.current?.focus();
        return () => previousFocus?.focus?.();
    }, []);
    const targets = plan?.targets.filter(target => !selectable || selected.has(target.uid)) || [];
    const execute = async () => {
        if (!plan || plan.mode !== mode || stale || executing.current || targets.length === 0) return;
        executing.current = true;
        setRunning(true); setError('');
        try {
            setResults(await ExecuteResourceAction({ ...plan, targets }));
        } catch (err) {
            // A timeout may have followed a successful write. Require a fresh
            // preview before another attempt instead of replaying blindly.
            setError(`${String(err)}. Refresh the preview before trying again; a request may already have been accepted.`);
            setPlan(null);
        } finally { executing.current = false; setRunning(false); }
    };
    return createPortal(
        <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/50" onClick={() => { if (!running) onClose(); }}>
            <div ref={dialogRef} tabIndex={-1} onKeyDown={event => {
                if (event.key === 'Escape' && !running) { event.stopPropagation(); onClose(); }
                if (event.key === 'Tab') {
                    const controls = Array.from(event.currentTarget.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled)'));
                    if (!controls.length) { event.preventDefault(); return; }
                    const first = controls[0]; const last = controls[controls.length - 1];
                    if (event.shiftKey && (document.activeElement === first || document.activeElement === event.currentTarget)) { event.preventDefault(); last?.focus(); }
                    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
                }
            }} role="dialog" aria-modal="true" aria-labelledby="operator-action-title" className="bg-surface-light border border-border rounded-lg shadow-xl w-[36rem] max-w-full max-h-[85vh] overflow-y-auto m-4 p-5 space-y-4" onClick={event => event.stopPropagation()}>
                <h2 id="operator-action-title" className="text-lg font-medium text-white">{action.label.replace(/…$/, '')}</h2>
                <p className="text-sm text-gray-400 break-all">{source.context} · {source.namespace ? `${source.namespace}/` : ''}{source.name}</p>
                <p className="text-sm text-gray-300">{action.description}</p>
                {stale && <p role="alert" className="text-yellow-400 text-sm">Switch back to {source.context} to run this action.</p>}
                {!results && <>
                    {action.modes.length > 1 && <fieldset disabled={running} className="flex gap-4 text-sm text-gray-300">
                        <legend className="sr-only">Action scope</legend>
                        {action.modes.map(option => <label key={option.id} className="flex items-center gap-2">
                            <input type="radio" name="operator-action-mode" checked={mode === option.id} onChange={() => { setPlan(null); setMode(option.id); }} />{option.label}
                        </label>)}
                    </fieldset>}
                    {loading && <p role="status" className="text-sm text-gray-400">Discovering affected resources…</p>}
                    {plan && <>
                        <p className="text-sm text-gray-400">{plan.summary}</p>
                        <div className="text-sm text-gray-300 space-y-2 max-h-64 overflow-y-auto">
                            {plan.targets.map(target => <label key={target.uid} className="flex gap-2 items-center break-all">
                                {selectable && <input type="checkbox" disabled={running} checked={selected.has(target.uid)} onChange={event => {
                                    const next = new Set(selected);
                                    if (event.target.checked) next.add(target.uid); else next.delete(target.uid);
                                    setSelected(next);
                                }} />}
                                <span>{target.kind}: {target.namespace ? `${target.namespace}/` : ''}{target.name}{target.pending ? ' — already in requested state' : ''}</span>
                            </label>)}
                        </div>
                        <p className="text-xs text-gray-400">{targets.length} resource{targets.length === 1 ? '' : 's'} affected</p>
                    </>}
                </>}
                {error && <p role="alert" className="text-sm text-red-400 break-words">{error}</p>}
                {results && <div role="status" className="space-y-2 text-sm">
                    {results.map(result => <p key={result.target.uid} className={result.status === 'failed' ? 'text-red-400' : 'text-gray-300'}>
                        {result.target.namespace ? `${result.target.namespace}/` : ''}{result.target.name}: {result.status === 'requested' ? 'Request accepted' : result.status === 'pending' ? 'Already in requested state' : result.error}{result.message ? ` · ${result.message}` : ''}
                    </p>)}
                    <p className="text-gray-400">Accepted requests are processed by the operator. Check resource status and events for completion.</p>
                </div>}
                <div className="flex justify-end gap-2">
                    <button disabled={running} onClick={onClose} className="px-3 py-2 text-sm text-gray-300 hover:bg-surface-hover rounded disabled:opacity-50">{results ? 'Close' : 'Cancel'}</button>
                    {!results && !loading && !running && <button onClick={() => { setPlan(null); setLoading(true); setRevision(value => value + 1); }} className="px-3 py-2 text-sm text-gray-300 hover:bg-surface-hover rounded">Refresh preview</button>}
                    {!results && <button disabled={!plan || loading || running || stale || targets.length === 0} onClick={execute} className="px-3 py-2 text-sm bg-primary text-white rounded disabled:opacity-50">{running ? 'Requesting…' : 'Confirm request'}</button>}
                </div>
            </div>
        </div>, document.body,
    );
}
