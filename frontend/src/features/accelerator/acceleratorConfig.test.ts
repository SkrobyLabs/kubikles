import { describe, expect, it } from 'vitest';
import { acceleratorPolicy, removeAcceleratorOverride, renameAcceleratorOverride, upsertAcceleratorOverride } from './acceleratorConfig';

const settings = { enabledByDefault: false, defaultNamespace: 'shared', connectionOverrides: [] };

describe('accelerator connection policy', () => {
  it('uses the Settings namespace before the context namespace', () => {
    expect(acceleratorPolicy(settings, 'prod', 'context-ns')).toMatchObject({ enabled: false, namespace: 'shared', namespaceSource: 'Kubikles Settings' });
  });
  it('treats an empty connection override as global default inheritance', () => {
    const next = upsertAcceleratorOverride(settings, { contextName: 'prod', enabled: true, namespace: '' });
    expect(acceleratorPolicy(next, 'prod', 'context-ns')).toMatchObject({ enabled: true, namespace: 'shared', namespaceSource: 'Kubikles Settings' });
  });
  it('uses kubikles-app when no namespace is configured', () => {
    const inherited = { ...settings, defaultNamespace: '', connectionOverrides: [] };
    expect(acceleratorPolicy(inherited, 'prod', '*')).toMatchObject({ namespace: 'kubikles-app', namespaceSource: 'Kubikles Settings' });
  });
  it('keeps the explicit all-namespaces marker as context namespace inheritance', () => {
    const inherited = { ...settings, defaultNamespace: '', connectionOverrides: [] };
    expect(acceleratorPolicy({ ...inherited, connectionOverrides: [{ contextName: 'prod', namespace: '*' }] }, 'prod', 'context-ns')).toMatchObject({ namespace: 'context-ns', namespaceSource: 'context namespace' });
    expect(acceleratorPolicy({ ...inherited, defaultNamespace: '*' }, 'prod', 'context-ns')).toMatchObject({ namespace: 'context-ns', namespaceSource: 'context namespace' });
  });
  it('updates only the selected context and supports rename and reset', () => {
    const saved = upsertAcceleratorOverride({ ...settings, connectionOverrides: [{ contextName: 'keep', enabled: true }] }, { contextName: 'old', namespace: 'target' });
    const renamed = renameAcceleratorOverride(saved, 'old', 'new');
    expect(renamed.connectionOverrides.map(item => item.contextName)).toEqual(['keep', 'new']);
    expect(removeAcceleratorOverride(renamed, 'new').connectionOverrides).toEqual([{ contextName: 'keep', enabled: true }]);
  });
});
