import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { ViewHandle } from './viewClient';
import { PARK_MAX, PARK_MS, clearParkedViews, parkKey, parkView, peekParkedView, takeParkedView } from './viewParking';

function fakeHandle(status = 'open') {
    const h = { status, close: vi.fn(), setActive: vi.fn(), getSnapshot: () => ({ status: h.status }) };
    return h as typeof h & ViewHandle;
}

describe('viewParking', () => {
    beforeEach(() => { vi.useFakeTimers(); });
    afterEach(() => { clearParkedViews(); vi.useRealTimers(); });

    it('hands a parked view back, inactive and still open', () => {
        const h = fakeHandle();
        const key = parkKey('ctx', 'pods');
        parkView(key, h);
        expect(h.setActive).toHaveBeenCalledWith(false);
        expect(peekParkedView(key)).toBe(h);
        expect(takeParkedView(key)).toBe(h);
        expect(takeParkedView(key)).toBeNull();
        vi.advanceTimersByTime(PARK_MS * 2);
        expect(h.close).not.toHaveBeenCalled();
    });

    it('closes a parked view when its time is up', () => {
        const h = fakeHandle();
        parkView(parkKey('ctx', 'pods'), h);
        vi.advanceTimersByTime(PARK_MS + 1);
        expect(h.close).toHaveBeenCalledTimes(1);
        expect(peekParkedView(parkKey('ctx', 'pods'))).toBeNull();
    });

    it('does not keep or reuse a view that is not open', () => {
        const failed = fakeHandle('failed');
        parkView(parkKey('ctx', 'pods'), failed);
        expect(failed.close).toHaveBeenCalledTimes(1);

        const h = fakeHandle();
        parkView(parkKey('ctx', 'nodes'), h);
        h.status = 'closed'; // the backend closed it (context switch) while parked
        expect(peekParkedView(parkKey('ctx', 'nodes'))).toBeNull();
        expect(takeParkedView(parkKey('ctx', 'nodes'))).toBeNull();
        expect(h.close).toHaveBeenCalledTimes(1);
    });

    it('keeps contexts apart and replaces an older view of the same list', () => {
        const a = fakeHandle(); const b = fakeHandle(); const other = fakeHandle();
        parkView(parkKey('ctx', 'pods'), a);
        parkView(parkKey('other', 'pods'), other);
        parkView(parkKey('ctx', 'pods'), b);
        expect(a.close).toHaveBeenCalledTimes(1);
        expect(takeParkedView(parkKey('ctx', 'pods'))).toBe(b);
        expect(takeParkedView(parkKey('other', 'pods'))).toBe(other);
    });

    it('closes the view parked longest ago when the limit is reached', () => {
        const handles = Array.from({ length: PARK_MAX + 1 }, () => fakeHandle());
        handles.forEach((h, i) => parkView(parkKey('ctx', `t${i}`), h));
        expect(handles[0].close).toHaveBeenCalledTimes(1);
        expect(handles[1].close).not.toHaveBeenCalled();
        expect(peekParkedView(parkKey('ctx', `t${PARK_MAX}`))).toBe(handles[PARK_MAX]);
    });
});
