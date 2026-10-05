import React, { useEffect, useId, useMemo, useRef, useState } from 'react';
import { createClockFormatter, filterClockTimeZones, getClockTimeZones } from './clockTimeZones';

export default function LogClock() {
    const [timeZone, setTimeZone] = useState('UTC');
    const [now, setNow] = useState(() => new Date());
    const [query, setQuery] = useState('');
    const dialog = useRef<HTMLDialogElement>(null);
    const titleId = useId();
    const searchId = useId();
    const systemZone = useMemo(() => Intl.DateTimeFormat().resolvedOptions().timeZone, []);
    const zones = useMemo(() => getClockTimeZones(systemZone), [systemZone]);
    const formatter = useMemo(() => createClockFormatter(timeZone), [timeZone]);
    const matches = filterClockTimeZones(zones, query);

    useEffect(() => {
        const timer = window.setInterval(() => setNow(new Date()), 1000);
        return () => window.clearInterval(timer);
    }, []);

    return (
        <div className="ml-auto pl-3 shrink-0">
            <button
                type="button"
                className="rounded px-2 py-1.5 text-xs text-gray-300 hover:text-white hover:bg-white/10 tabular-nums focus-visible:outline focus-visible:outline-primary"
                aria-label={`${formatter.format(now)} ${timeZone}. Change clock timezone`}
                aria-haspopup="dialog"
                onClick={() => { setQuery(''); dialog.current?.showModal(); }}
            >
                <time dateTime={now.toISOString()}>{formatter.format(now)} · {timeZone.replace(/_/g, ' ')}</time>
            </button>
            <dialog ref={dialog} aria-labelledby={titleId} className="w-96 max-w-[90vw] rounded-lg border border-border bg-surface p-4 text-gray-200 backdrop:bg-black/50">
                <div className="flex items-center justify-between mb-3">
                    <h2 id={titleId} className="font-medium">Clock timezone</h2>
                    <button type="button" onClick={() => dialog.current?.close()} className="rounded px-2 py-1 hover:bg-white/10">Close</button>
                </div>
                <label htmlFor={searchId} className="block text-xs text-gray-400 mb-1">Filter timezones</label>
                <input id={searchId} autoFocus type="search" value={query} onChange={event => setQuery(event.target.value)} placeholder="Search city or region…" className="w-full rounded border border-border bg-background px-3 py-2 mb-2 text-sm" />
                <div className="max-h-64 overflow-y-auto">
                    {matches.map(zone => (
                        <button key={zone} type="button" aria-pressed={zone === timeZone} className={`block w-full rounded px-3 py-2 text-left text-sm hover:bg-white/10 ${zone === timeZone ? 'bg-primary/20 text-primary' : ''}`} onClick={() => { setTimeZone(zone); dialog.current?.close(); }}>
                            {zone.replace(/_/g, ' ')}{zone === systemZone ? ' (system)' : ''}
                        </button>
                    ))}
                    {matches.length === 0 && <p className="p-3 text-sm text-gray-400">No matching timezones</p>}
                </div>
            </dialog>
        </div>
    );
}
