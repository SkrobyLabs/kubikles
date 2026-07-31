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
    describe('Secret Direct methods retain their generated arities', () => {
        const directMethods = { ListSecretsMetadata: 2, GetSecretData: 2, GetSecretYaml: 2, CancelListRequest: 1, SubscribeResourceWatcher: 2, UnsubscribeWatcher: 1, UpdateSecretData: 3, UpdateSecretYaml: 3, DeleteSecret: 2 };
        Object.entries(directMethods).forEach(([name, arity]) => {
            it(`${name} takes ${arity} args`, () => expect(getWailsFunctionArity(wailsContent, name)).toBe(arity));
        });
    });
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
            'UninstallHelmRelease',
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

describe('Secret Direct source contract', () => {
    const editor = readFileSync(join(__dirname, '../components/shared/SecretEditor.tsx'), 'utf-8');
    const actions = readFileSync(join(__dirname, '../features/config/secrets/useSecretActions.tsx'), 'utf-8');
    const list = readFileSync(join(__dirname, '../features/config/secrets/SecretList.tsx'), 'utf-8');
    it('keeps exact Direct mutation call sites and argument counts', () => {
        expect(editor).toMatch(/UpdateSecretYaml\(namespace, resourceName, yamlContent\)/);
        expect(editor).toMatch(/UpdateSecretData\(namespace, resourceName, dataToSave\)/);
        expect(editor.match(/UpdateSecretYaml\(/g)).toHaveLength(1);
        expect(editor.match(/UpdateSecretData\(/g)).toHaveLength(1);
        expect(actions).toMatch(/DeleteSecret\(namespace, name\)/);
        expect(actions.match(/DeleteSecret\(/g)).toHaveLength(1);
        expect(list).toMatch(/deleteApi:\s*DeleteSecret/);
    });
    it('does not introduce Accelerator or router symbols to Secret behavior', () => {
        for (const source of [editor, actions, list]) expect(source).not.toMatch(/Accelerator|accelerator|Router|router/);
    });
});
