import { describe, it, expect } from 'vitest';
import { priorityClassFields } from './priorityclasses';

describe('priorityClassFields', () => {
    describe('value', () => {
        it('extracts priority value', () => {
            expect(priorityClassFields.value.extractor({ value: 1000000 })).toBe('1000000');
            expect(priorityClassFields.value.extractor({})).toBe('0');
        });
    });

    describe('globaldefault', () => {
        it('returns global default status', () => {
            expect(priorityClassFields.globaldefault.extractor({ globalDefault: true })).toBe('true');
            expect(priorityClassFields.globaldefault.extractor({ globalDefault: false })).toBe('false');
            expect(priorityClassFields.globaldefault.extractor({})).toBe('false');
        });
    });

    describe('preemption', () => {
        it('extracts preemption policy', () => {
            expect(priorityClassFields.preemption.extractor({ preemptionPolicy: 'Never' })).toBe('Never');
            expect(priorityClassFields.preemption.extractor({})).toBe('PreemptLowerPriority');
        });
    });

    describe('description', () => {
        it('extracts description', () => {
            expect(priorityClassFields.description.extractor({ description: 'High priority class' })).toBe('High priority class');
            expect(priorityClassFields.description.extractor({})).toBe('');
        });
    });
});
