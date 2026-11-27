/**
 * Tracks whether a followed log view is pinned to its end.
 *
 * The list's own "at bottom" signal is not reliable for this: a burst of
 * appended lines grows the content faster than the list scrolls, the view is
 * then reported as not at the bottom, and following would stop for good. The
 * pin instead changes only on user input: scrolling away releases it, and
 * scrolling back to the end (or the list reporting the end) restores it.
 */

/** Distance from the end, in pixels, still treated as "at the end" after user scrolling. */
export const PIN_SLACK_PX = 48;
/** A scroll event this soon after wheel, touch or key input is attributed to the user. */
export const USER_INPUT_WINDOW_MS = 400;

export interface FollowPin {
    isPinned(): boolean;
    pin(): void;
    unpin(): void;
    /** Wheel input; scrolling up releases the pin at once. */
    noteWheel(deltaY: number): void;
    /**
     * Touch or keyboard input that may scroll the list. 'down' can only move towards the end;
     * 'up' moves away from it and releases the pin at once, like wheeling up.
     */
    noteUserInput(direction?: 'down' | 'up'): void;
    /** Pointer held down on the list (scrollbar drag). */
    setHeld(held: boolean): void;
    /** A scroll event, with the distance left to the end of the content. */
    noteScroll(gapPx: number): void;
    /** The list's own at-bottom signal; only a true value is trusted. */
    noteAtBottom(atBottom: boolean): void;
}

export function createFollowPin(now: () => number = () => performance.now()): FollowPin {
    let pinned = true;
    let held = false;
    let lastInput = Number.NEGATIVE_INFINITY;
    // After input that moves towards the end, scroll events may restore the pin but not release it:
    // the list re-measures rows as it lands and briefly reports a gap that the user did not make.
    let towardsEnd = false;
    // After input that moves away from the end, scroll events may release the pin but not restore
    // it: one arrow key or a short wheel tick stays inside the slack, and re-pinning there would
    // snap a busy log straight back to its end.
    let awayFromEnd = false;
    return {
        isPinned: () => pinned,
        pin: () => { pinned = true; },
        unpin: () => { pinned = false; },
        noteWheel: (deltaY) => {
            lastInput = now();
            towardsEnd = deltaY > 0;
            awayFromEnd = deltaY < 0;
            if (deltaY < 0) pinned = false;
        },
        noteUserInput: (direction) => {
            lastInput = now();
            towardsEnd = direction === 'down';
            awayFromEnd = direction === 'up';
            if (direction === 'up') pinned = false;
        },
        setHeld: (value) => {
            held = value;
            if (value) {
                lastInput = now();
                towardsEnd = false;
                awayFromEnd = false;
            }
        },
        noteScroll: (gapPx) => {
            if (!held && now() - lastInput > USER_INPUT_WINDOW_MS) return;
            if (gapPx <= PIN_SLACK_PX) {
                if (!awayFromEnd) pinned = true;
            } else if (held || !towardsEnd) pinned = false;
        },
        noteAtBottom: (atBottom) => {
            if (atBottom) pinned = true;
        },
    };
}

/** Which way a key scrolls a list, or null for keys that do not scroll it. */
export function followKeyDirection(key: string, shift: boolean): 'up' | 'down' | null {
    switch (key) {
        case 'ArrowUp': case 'PageUp': case 'Home': return 'up';
        case 'ArrowDown': case 'PageDown': case 'End': return 'down';
        case ' ': return shift ? 'up' : 'down';
        default: return null;
    }
}
