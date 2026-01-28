import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));

/**
 * Extracts exported function names from a JavaScript file.
 * Matches patterns like: export function FunctionName(
 */
function extractExportedFunctions(content) {
    const regex = /^export function (\w+)\s*\(/gm;
    const functions = [];
    let match;
    while ((match = regex.exec(content)) !== null) {
        functions.push(match[1]);
    }
    return functions.sort();
}

/**
 * Methods that exist in the adapter but not in Wails, with justification.
 * Add entries here only when there's a legitimate reason for the difference.
 *
 * Format: { methodName: "reason for the difference" }
 */
const ALLOWED_EXTRA_METHODS = {
    // Example:
    // 'ServerOnlyMethod': 'Only available in server mode, no Wails equivalent',
};

describe('App.js Adapter Sync', () => {
    it('should contain all methods from Wails-generated App.js', () => {
        // Read both files
        const wailsAppPath = join(__dirname, '../../../../../wailsjs/go/main/App.js');
        const adapterAppPath = join(__dirname, 'App.js');

        const wailsContent = readFileSync(wailsAppPath, 'utf-8');
        const adapterContent = readFileSync(adapterAppPath, 'utf-8');

        // Extract function names
        const wailsFunctions = extractExportedFunctions(wailsContent);
        const adapterFunctions = extractExportedFunctions(adapterContent);

        // Find functions in Wails that are missing from adapter
        const missingInAdapter = wailsFunctions.filter(fn => !adapterFunctions.includes(fn));

        // The test fails if any Wails methods are missing from the adapter
        expect(missingInAdapter,
            `Adapter is missing ${missingInAdapter.length} method(s) from Wails bindings.\n` +
            `Add these to the adapter:\n${missingInAdapter.map(fn => `  export function ${fn}(...args) { return AppProxy.${fn}(...args); }`).join('\n')}`
        ).toHaveLength(0);
    });

    it('should not have extra methods unless explicitly allowed', () => {
        // Read both files
        const wailsAppPath = join(__dirname, '../../../../../wailsjs/go/main/App.js');
        const adapterAppPath = join(__dirname, 'App.js');

        const wailsContent = readFileSync(wailsAppPath, 'utf-8');
        const adapterContent = readFileSync(adapterAppPath, 'utf-8');

        // Extract function names
        const wailsFunctions = extractExportedFunctions(wailsContent);
        const adapterFunctions = extractExportedFunctions(adapterContent);

        // Find functions in adapter that don't exist in Wails
        const extraInAdapter = adapterFunctions.filter(fn => !wailsFunctions.includes(fn));

        // Filter out explicitly allowed extras
        const unexpectedExtras = extraInAdapter.filter(fn => !ALLOWED_EXTRA_METHODS[fn]);

        // The test fails if there are unexpected extra methods
        expect(unexpectedExtras,
            `Adapter has ${unexpectedExtras.length} unexpected extra method(s) not in Wails bindings.\n` +
            `Either:\n` +
            `  1. Remove stale methods from the adapter\n` +
            `  2. Add to ALLOWED_EXTRA_METHODS with justification\n\n` +
            `Extra methods:\n${unexpectedExtras.map(fn => `  - ${fn}`).join('\n')}`
        ).toHaveLength(0);
    });

    it('should have Wails-generated App.js file available', () => {
        const wailsAppPath = join(__dirname, '../../../../../wailsjs/go/main/App.js');

        expect(() => {
            readFileSync(wailsAppPath, 'utf-8');
        }).not.toThrow();
    });
});
