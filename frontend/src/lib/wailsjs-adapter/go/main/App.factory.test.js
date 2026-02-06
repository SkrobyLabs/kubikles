import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const frontendRoot = join(__dirname, '../../../../..');

/**
 * Extracts exported function names and their parameter counts from App.js.
 * Matches patterns like: export function FunctionName(arg1, arg2) {
 */
function extractFunctionSignatures(content) {
    const regex = /^export function (\w+)\s*\(([^)]*)\)/gm;
    const signatures = {};
    let match;
    while ((match = regex.exec(content)) !== null) {
        const name = match[1];
        const params = match[2].trim();
        const paramCount = params ? params.split(',').length : 0;
        signatures[name] = paramCount;
    }
    return signatures;
}

/**
 * Parses hooks/resources/index.ts and extracts all factory registrations.
 * Returns: { namespaced: ['ListPods', ...], clusterScoped: ['ListNodes', ...] }
 */
function extractFactoryRegistrations(content) {
    const namespaced = [];
    const clusterScoped = [];

    // Match: createNamespacedResourceHook('type', FunctionRef, 'state')
    const namespacedRegex = /createNamespacedResourceHook\s*\(\s*['"][^'"]+['"]\s*,\s*(\w+)\s*,/g;
    let match;
    while ((match = namespacedRegex.exec(content)) !== null) {
        namespaced.push(match[1]);
    }

    // Match: createClusterScopedResourceHook('type', FunctionRef, 'state')
    const clusterRegex = /createClusterScopedResourceHook\s*\(\s*['"][^'"]+['"]\s*,\s*(\w+)\s*,/g;
    while ((match = clusterRegex.exec(content)) !== null) {
        clusterScoped.push(match[1]);
    }

    return { namespaced, clusterScoped };
}

describe('Resource Hook Factory Registration Validation', () => {
    const wailsAppPath = join(frontendRoot, 'wailsjs/go/main/App.js');
    const wailsContent = readFileSync(wailsAppPath, 'utf-8');
    const signatures = extractFunctionSignatures(wailsContent);

    const resourceIndexPath = join(frontendRoot, 'src/hooks/resources/index.ts');
    const resourceIndexContent = readFileSync(resourceIndexPath, 'utf-8');
    const registrations = extractFactoryRegistrations(resourceIndexContent);

    it('should register namespaced hooks only with 2-parameter functions (requestId, namespace)', () => {
        const violations = [];

        for (const fnName of registrations.namespaced) {
            const paramCount = signatures[fnName];
            if (paramCount === undefined) {
                violations.push(`  ${fnName} - not found in Wails bindings`);
            } else if (paramCount !== 2) {
                violations.push(
                    `  ${fnName} - has ${paramCount} param(s) in Wails bindings, expected 2 (requestId, namespace)`
                );
            }
        }

        expect(violations,
            `Found ${violations.length} namespaced hook registration(s) with wrong parameter count:\n\n` +
            `${violations.join('\n')}\n\n` +
            `createNamespacedResourceHook calls listFn(requestId, namespace) - the Go function must accept exactly 2 string params.\n` +
            `Fix the Go function signature or move this resource to createClusterScopedResourceHook.`
        ).toHaveLength(0);
    });

    it('should register cluster-scoped hooks only with 1-parameter functions (requestId)', () => {
        const violations = [];

        for (const fnName of registrations.clusterScoped) {
            const paramCount = signatures[fnName];
            if (paramCount === undefined) {
                violations.push(`  ${fnName} - not found in Wails bindings`);
            } else if (paramCount !== 1) {
                violations.push(
                    `  ${fnName} - has ${paramCount} param(s) in Wails bindings, expected 1 (requestId)`
                );
            }
        }

        expect(violations,
            `Found ${violations.length} cluster-scoped hook registration(s) with wrong parameter count:\n\n` +
            `${violations.join('\n')}\n\n` +
            `createClusterScopedResourceHook calls listFn(requestId) - the Go function must accept exactly 1 string param.\n` +
            `Fix the Go function signature or move this resource to createNamespacedResourceHook.`
        ).toHaveLength(0);
    });

    it('should have parsed factory registrations', () => {
        // Sanity check: we found a reasonable number of registrations
        expect(registrations.namespaced.length).toBeGreaterThan(10);
        expect(registrations.clusterScoped.length).toBeGreaterThan(5);

        // Verify specific known registrations
        expect(registrations.namespaced).toContain('ListPods');
        expect(registrations.namespaced).toContain('ListDeployments');
        expect(registrations.namespaced).toContain('ListHPAs');
        expect(registrations.clusterScoped).toContain('ListNodes');
        expect(registrations.clusterScoped).toContain('ListNamespaces');
        expect(registrations.clusterScoped).toContain('ListClusterRoles');
    });
});
