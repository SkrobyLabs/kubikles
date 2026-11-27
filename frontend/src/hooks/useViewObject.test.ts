import { beforeEach, describe, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ addNotification: vi.fn() }));
vi.mock('react', () => ({ useCallback: (fn: any) => fn }));
vi.mock('../context', () => ({ useNotification: () => ({ addNotification: mocks.addNotification }) }));

import { useWithFullObject } from './useViewObject';
import type { RowItem } from '../lib/views/rowItem';

const row: RowItem = { metadata: { uid: 'u1', name: 'e1', namespace: 'ns' }, cells: {}, __seq: 1, __view: true };
const flush = async () => { for (let i = 0; i < 5; i++) await Promise.resolve(); };

beforeEach(() => mocks.addNotification.mockReset());

describe('useWithFullObject', () => {
    it('fetches the object at click time and hands it to the handler, with the other arguments', async () => {
        const view: any = { getObject: vi.fn().mockResolvedValue({ spec: { x: 1 } }) };
        const handler = vi.fn();
        useWithFullObject(view)(handler)(row, 'extra');
        expect(handler).not.toHaveBeenCalled();
        expect(view.getObject).toHaveBeenCalledWith(row);
        await flush();
        expect(handler).toHaveBeenCalledWith({ spec: { x: 1 } }, 'extra');
    });

    it('calls the handler directly for legacy items, and when there is no view', () => {
        const handler = vi.fn();
        const legacy = { metadata: { uid: 'x', name: 'n' }, spec: {} };
        const view: any = { getObject: vi.fn() };
        useWithFullObject(view)(handler)(legacy);
        expect(handler).toHaveBeenCalledWith(legacy);
        expect(view.getObject).not.toHaveBeenCalled();
        useWithFullObject(null)(handler)(row);
        expect(handler).toHaveBeenLastCalledWith(row);
    });

    it('reports a failed fetch and does not call the handler with a half object', async () => {
        const view: any = { getObject: vi.fn().mockRejectedValue(new Error('not found')) };
        const handler = vi.fn();
        useWithFullObject(view)(handler)(row);
        await flush();
        expect(handler).not.toHaveBeenCalled();
        expect(mocks.addNotification).toHaveBeenCalledWith(expect.objectContaining({ type: 'error', message: 'not found' }));
    });
});
