import { describe, it, expect } from 'vitest';

/**
 * Tests for SearchSelect display logic.
 * These test the core display functions without rendering the component.
 */

// Replicate the getDisplayLabel logic from SearchSelect
const getDisplayLabel = (option, getOptionLabel = null) => {
    if (getOptionLabel) return getOptionLabel(option);
    return option === '' ? 'All Namespaces' : String(option);
};

// Replicate the getSelectedOptionDisplay logic from SearchSelect
const getSelectedOptionDisplay = (value, options, placeholder, getOptionValue = null, getOptionLabel = null) => {
    if (value === undefined || value === null) return placeholder;

    // If using object options, find the matching option
    if (getOptionValue) {
        const selectedOption = options.find(opt => getOptionValue(opt) === value);
        return selectedOption ? getDisplayLabel(selectedOption, getOptionLabel) : placeholder;
    }

    // For string options: if value is empty and not in options, show placeholder
    if (value === '' && !options.includes('')) {
        return placeholder;
    }

    return getDisplayLabel(value, getOptionLabel);
};

describe('SearchSelect display logic', () => {
    describe('getDisplayLabel', () => {
        it('returns "All Namespaces" for empty string', () => {
            expect(getDisplayLabel('')).toBe('All Namespaces');
        });

        it('returns string value for non-empty strings', () => {
            expect(getDisplayLabel('default')).toBe('default');
            expect(getDisplayLabel('kube-system')).toBe('kube-system');
        });

        it('uses custom getOptionLabel when provided', () => {
            const customLabel = (opt) => `NS: ${opt}`;
            expect(getDisplayLabel('default', customLabel)).toBe('NS: default');
            expect(getDisplayLabel('', customLabel)).toBe('NS: ');
        });
    });

    describe('getSelectedOptionDisplay', () => {
        it('returns placeholder for null/undefined values', () => {
            expect(getSelectedOptionDisplay(null, [], 'Select...')).toBe('Select...');
            expect(getSelectedOptionDisplay(undefined, [], 'Select...')).toBe('Select...');
        });

        it('returns placeholder for empty string when not in options', () => {
            // This is the key fix - resource name selector with empty value
            const options = ['pod-1', 'pod-2', 'pod-3'];
            expect(getSelectedOptionDisplay('', options, 'Select resource...')).toBe('Select resource...');
        });

        it('returns "All Namespaces" for empty string when in options', () => {
            // Namespace selector includes empty string as an option
            const options = ['', 'default', 'kube-system'];
            expect(getSelectedOptionDisplay('', options, 'Select...')).toBe('All Namespaces');
        });

        it('returns selected value for non-empty strings', () => {
            const options = ['default', 'kube-system'];
            expect(getSelectedOptionDisplay('default', options, 'Select...')).toBe('default');
        });

        it('handles object options with getOptionValue/getOptionLabel', () => {
            const options = [
                { value: 'deployment', label: 'Deployment' },
                { value: 'pod', label: 'Pod' }
            ];
            const getOptionValue = (opt) => opt.value;
            const getOptionLabel = (opt) => opt.label;

            expect(getSelectedOptionDisplay('deployment', options, 'Select...', getOptionValue, getOptionLabel))
                .toBe('Deployment');
            expect(getSelectedOptionDisplay('pod', options, 'Select...', getOptionValue, getOptionLabel))
                .toBe('Pod');
        });

        it('returns placeholder for object options when value not found', () => {
            const options = [
                { value: 'deployment', label: 'Deployment' }
            ];
            const getOptionValue = (opt) => opt.value;
            const getOptionLabel = (opt) => opt.label;

            expect(getSelectedOptionDisplay('unknown', options, 'Select...', getOptionValue, getOptionLabel))
                .toBe('Select...');
        });
    });
});

describe('Namespace dropdown behavior', () => {
    const namespaceOptions = ['', 'default', 'kube-system', 'monitoring'];

    it('shows "All Namespaces" when empty string selected', () => {
        expect(getSelectedOptionDisplay('', namespaceOptions, 'Select namespace...'))
            .toBe('All Namespaces');
    });

    it('shows namespace name when specific namespace selected', () => {
        expect(getSelectedOptionDisplay('default', namespaceOptions, 'Select namespace...'))
            .toBe('default');
        expect(getSelectedOptionDisplay('monitoring', namespaceOptions, 'Select namespace...'))
            .toBe('monitoring');
    });
});

describe('Resource name dropdown behavior', () => {
    const resourceOptions = ['nginx-deployment', 'redis-deployment', 'api-server'];

    it('shows placeholder when no resource selected (empty string)', () => {
        expect(getSelectedOptionDisplay('', resourceOptions, 'Select resource...'))
            .toBe('Select resource...');
    });

    it('shows resource name when selected', () => {
        expect(getSelectedOptionDisplay('nginx-deployment', resourceOptions, 'Select resource...'))
            .toBe('nginx-deployment');
    });

    it('shows placeholder when empty options array', () => {
        expect(getSelectedOptionDisplay('', [], 'Select resource...'))
            .toBe('Select resource...');
    });
});
