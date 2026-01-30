import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));

// Read the Wails-generated App.js file directly to verify function signatures.
// We can't import via ES modules because Vite aliases wailsjs imports to the adapter.
// The adapter uses (...args) which has length 0, so we parse the source instead.
//
// These tests verify that Wails-generated bindings match expected arity.
// If a Go function signature changes, these tests catch mismatches before
// they cause silent runtime bugs.
//
// Bug context: useBulkActions previously passed (context, namespace, name) but
// most Go Delete functions only accepted (namespace, name). The context string
// was silently treated as namespace, causing deletes to target the wrong resource.

/**
 * Extracts the number of arguments for a function from the Wails-generated App.js
 */
function getWailsFunctionArity(content, fnName) {
    // Match: export function FnName(arg1, arg2, ...) {
    const regex = new RegExp(`export function ${fnName}\\(([^)]*)\\)`, 'm');
    const match = content.match(regex);
    if (!match) return null;
    const args = match[1].trim();
    if (args === '') return 0;
    return args.split(',').length;
}

// Read the Wails-generated file once
const wailsAppPath = join(__dirname, '../../wailsjs/go/main/App.js');
const wailsContent = readFileSync(wailsAppPath, 'utf-8');

describe('Wails binding signatures for delete/restart operations', () => {
    describe('namespaced delete functions take exactly 2 args (namespace, name)', () => {
        const namespacedDeleteFns = [
            'DeletePod',
            'ForceDeletePod',
            'DeleteDeployment',
            'DeleteStatefulSet',
            'DeleteDaemonSet',
            'DeleteService',
            'DeleteConfigMap',
            'DeleteSecret',
            'DeleteCronJob',
            'DeleteJob',
            'DeleteReplicaSet',
            'DeleteIngress',
            'DeleteHPA',
            'DeletePDB',
            'DeletePVC',
            'DeleteRole',
            'DeleteRoleBinding',
            'DeleteServiceAccount',
            'DeleteNetworkPolicy',
            'DeleteResourceQuota',
            'DeleteLimitRange',
            'DeleteEndpoints',
            'DeleteEndpointSlice',
            'DeleteLease',
            'DeleteEvent',
        ];

        namespacedDeleteFns.forEach((name) => {
            it(`${name} takes 2 args`, () => {
                const arity = getWailsFunctionArity(wailsContent, name);
                expect(arity, `Function ${name} not found in Wails bindings`).not.toBeNull();
                expect(arity).toBe(2);
            });
        });
    });

    describe('cluster-scoped delete functions take exactly 1 arg (name)', () => {
        const clusterScopedDeleteFns = [
            'DeleteNamespace',
            'DeleteNode',
            'DeletePV',
            'DeleteStorageClass',
            'DeleteClusterRole',
            'DeleteClusterRoleBinding',
            'DeleteCRD',
            'DeleteCSIDriver',
            'DeleteCSINode',
            'DeleteIngressClass',
            'DeletePriorityClass',
            'DeleteMutatingWebhookConfiguration',
            'DeleteValidatingWebhookConfiguration',
        ];

        clusterScopedDeleteFns.forEach((name) => {
            it(`${name} takes 1 arg`, () => {
                const arity = getWailsFunctionArity(wailsContent, name);
                expect(arity, `Function ${name} not found in Wails bindings`).not.toBeNull();
                expect(arity).toBe(1);
            });
        });
    });

    describe('restart functions take exactly 2 args (namespace, name)', () => {
        const restartFns = [
            'RestartDeployment',
            'RestartStatefulSet',
            'RestartDaemonSet',
        ];

        restartFns.forEach((name) => {
            it(`${name} takes 2 args`, () => {
                const arity = getWailsFunctionArity(wailsContent, name);
                expect(arity, `Function ${name} not found in Wails bindings`).not.toBeNull();
                expect(arity).toBe(2);
            });
        });
    });
});
