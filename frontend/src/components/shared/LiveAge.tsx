import React, { useEffect, useReducer, useRef } from 'react';
import { AGE_SECONDS_UNTIL, formatAge, nextAgeChangeMs } from '~/utils/formatting';

/**
 * One character of a ticking age. Whether it animates is decided once, when the element is
 * created: characters present at the first paint never do (a list that mounts or scrolls must
 * not shimmer), and a character that replaces another later does. Deciding it per render
 * would add the animation class to every existing character on the first tick.
 */
const AgeChar = React.memo(function AgeChar({ ch, animate }: { ch: string; animate: boolean }) {
    const cls = useRef(animate ? 'age-digit' : 'age-char').current;
    return <span className={cls}>{ch}</span>;
}, (prev, next) => prev.ch === next.ch);

/**
 * An age that keeps itself current. The start time is known, so the elapsed time between two
 * updates from the cluster is computed locally and is exact; nothing here waits for data. The
 * cell re-renders only when its text is due to change: every second while seconds are shown,
 * then once a minute, then once an hour.
 *
 * While seconds are shown each character is its own element keyed by position and value, so a
 * digit that changes is replaced and settles in with a short fade (see .age-digit); digits that stay do not.
 * The characters are ordinary text for copying and for screen readers.
 */
export const LiveAge = React.memo(function LiveAge({ timestamp }: { timestamp: string }) {
    const [, tick] = useReducer((n: number) => n + 1, 0);
    const mounted = useRef(false);
    useEffect(() => {
        mounted.current = true;
        let id: ReturnType<typeof setTimeout>;
        const arm = () => {
            id = setTimeout(() => {
                // A hidden window skips the render; the next timer, or the list's own heartbeat, catches up.
                if (!document.hidden) tick();
                arm();
            }, nextAgeChangeMs(timestamp) + 5);
        };
        arm();
        return () => clearTimeout(id);
    }, [timestamp]);

    const text = formatAge(timestamp);
    const elapsed = Date.now() - new Date(timestamp).getTime();
    if (!(elapsed < AGE_SECONDS_UNTIL * 1000)) return <>{text}</>;
    return (
        <span className="tabular-nums">
            {Array.from(text, (ch, i) => <AgeChar key={`${i}${ch}`} ch={ch} animate={mounted.current} />)}
        </span>
    );
});

/**
 * The timestamp behind a cell whose rendered text is an age, or null. A column qualifies when its
 * text equals the age of the row's own value for that column (view rows) or of the creation time,
 * which keeps this independent of how each list names or builds its age column.
 */
export function ageTimestampFor(item: any, columnKey: string, content: unknown): string | null {
    if (typeof content !== 'string' || content === '') return null;
    const cell = item?.cells?.[columnKey];
    if (typeof cell === 'string' && formatAge(cell) === content) return cell;
    const created = item?.metadata?.creationTimestamp;
    if (columnKey === 'age' && typeof created === 'string' && formatAge(created) === content) return created;
    return null;
}
