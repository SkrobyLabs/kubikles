import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { generateId, loadViews, persistViews, STORAGE_KEY } from './useSavedViews';

describe('useSavedViews helpers', () => {
    describe('STORAGE_KEY', () => {
        it('has expected value', () => {
            expect(STORAGE_KEY).toBe('kubikles_savedviews');
        });
    });

    describe('generateId', () => {
        it('returns a string starting with view_', () => {
            const id = generateId();
            expect(typeof id).toBe('string');
            expect(id.startsWith('view_')).toBe(true);
        });

        it('generates unique IDs', () => {
            const ids = new Set();
            for (let i = 0; i < 100; i++) {
                ids.add(generateId());
            }
            expect(ids.size).toBe(100);
        });

        it('includes timestamp component', () => {
            const before = Date.now();
            const id = generateId();
            const after = Date.now();

            // ID format: view_{timestamp}_{random}
            const parts = id.split('_');
            expect(parts.length).toBe(3);

            const timestamp = parseInt(parts[1], 10);
            expect(timestamp).toBeGreaterThanOrEqual(before);
            expect(timestamp).toBeLessThanOrEqual(after);
        });

        it('includes random component', () => {
            const id = generateId();
            const parts = id.split('_');
            const randomPart = parts[2];

            // Random part should be alphanumeric and 9 chars
            expect(randomPart).toMatch(/^[a-z0-9]+$/);
            expect(randomPart.length).toBe(9);
        });
    });

    describe('loadViews', () => {
        const originalLocalStorage = global.localStorage;

        beforeEach(() => {
            // Mock localStorage
            const store = {};
            global.localStorage = {
                getItem: vi.fn((key) => store[key] || null),
                setItem: vi.fn((key, value) => { store[key] = value; }),
                removeItem: vi.fn((key) => { delete store[key]; }),
                clear: vi.fn(() => { Object.keys(store).forEach(k => delete store[k]); }),
            };
        });

        afterEach(() => {
            global.localStorage = originalLocalStorage;
        });

        it('returns empty array when no saved views', () => {
            const views = loadViews();
            expect(views).toEqual([]);
            expect(localStorage.getItem).toHaveBeenCalledWith(STORAGE_KEY);
        });

        it('returns parsed views from localStorage', () => {
            const savedViews = [
                { id: 'view_1', name: 'Running Pods', resourceType: 'pods' },
                { id: 'view_2', name: 'All Services', resourceType: 'services' }
            ];
            localStorage.getItem = vi.fn(() => JSON.stringify(savedViews));

            const views = loadViews();
            expect(views).toEqual(savedViews);
        });

        it('returns empty array on JSON parse error', () => {
            localStorage.getItem = vi.fn(() => 'invalid json{');
            const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

            const views = loadViews();

            expect(views).toEqual([]);
            expect(consoleSpy).toHaveBeenCalled();

            consoleSpy.mockRestore();
        });

        it('handles localStorage throwing error', () => {
            localStorage.getItem = vi.fn(() => { throw new Error('Storage quota exceeded'); });
            const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

            const views = loadViews();

            expect(views).toEqual([]);
            expect(consoleSpy).toHaveBeenCalled();

            consoleSpy.mockRestore();
        });
    });

    describe('persistViews', () => {
        const originalLocalStorage = global.localStorage;

        beforeEach(() => {
            const store = {};
            global.localStorage = {
                getItem: vi.fn((key) => store[key] || null),
                setItem: vi.fn((key, value) => { store[key] = value; }),
                removeItem: vi.fn((key) => { delete store[key]; }),
                clear: vi.fn(),
            };
        });

        afterEach(() => {
            global.localStorage = originalLocalStorage;
        });

        it('saves views to localStorage as JSON', () => {
            const views = [
                { id: 'view_1', name: 'Test View', resourceType: 'pods' }
            ];

            persistViews(views);

            expect(localStorage.setItem).toHaveBeenCalledWith(
                STORAGE_KEY,
                JSON.stringify(views)
            );
        });

        it('saves empty array', () => {
            persistViews([]);
            expect(localStorage.setItem).toHaveBeenCalledWith(STORAGE_KEY, '[]');
        });

        it('preserves complex view structures', () => {
            const views = [
                {
                    id: 'view_123_abc',
                    name: 'Complex View',
                    resourceType: 'pods',
                    query: 'status:Running namespace:prod',
                    namespace: ['prod', 'staging'],
                    hiddenColumns: ['age', 'labels'],
                    sortConfig: { key: 'name', direction: 'asc' },
                    columnFilters: { status: 'Running' },
                    createdAt: 1704067200000,
                    isDefault: true
                }
            ];

            persistViews(views);

            const savedJson = localStorage.setItem.mock.calls[0][1];
            expect(JSON.parse(savedJson)).toEqual(views);
        });

        it('handles localStorage error gracefully', () => {
            localStorage.setItem = vi.fn(() => { throw new Error('Quota exceeded'); });
            const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});

            // Should not throw
            expect(() => persistViews([{ id: '1' }])).not.toThrow();
            expect(consoleSpy).toHaveBeenCalled();

            consoleSpy.mockRestore();
        });
    });
});
