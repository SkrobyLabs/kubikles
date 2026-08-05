import type { AcceleratorConnectionOverride } from '~/context/ConfigContext';

export type AcceleratorSettings = { enabledByDefault: boolean; defaultNamespace: string; connectionOverrides: AcceleratorConnectionOverride[] };
export function acceleratorPolicy(settings: AcceleratorSettings, contextName: string, contextNamespace = 'default') {
  const override = settings.connectionOverrides.find(item => item.contextName === contextName);
  const namespace = override?.namespace === '' ? contextNamespace : (override?.namespace ?? settings.defaultNamespace ?? contextNamespace ?? 'default');
  return {
    enabled: override?.enabled ?? settings.enabledByDefault,
    enabledSource: override?.enabled === undefined ? 'Kubikles Settings' : 'connection override',
    namespace,
    namespaceSource: override?.namespace !== undefined ? (override.namespace === '' ? 'context namespace' : 'connection override') : (settings.defaultNamespace ? 'Kubikles Settings' : 'context namespace'),
    override,
  };
}
export function upsertAcceleratorOverride(settings: AcceleratorSettings, value: AcceleratorConnectionOverride): AcceleratorSettings {
  const rest = settings.connectionOverrides.filter(item => item.contextName !== value.contextName);
  return { ...settings, connectionOverrides: [...rest, value].sort((a, b) => a.contextName.localeCompare(b.contextName)) };
}
export function removeAcceleratorOverride(settings: AcceleratorSettings, contextName: string): AcceleratorSettings {
  return { ...settings, connectionOverrides: settings.connectionOverrides.filter(item => item.contextName !== contextName) };
}
export function renameAcceleratorOverride(settings: AcceleratorSettings, oldName: string, newName: string): AcceleratorSettings {
  const override = settings.connectionOverrides.find(item => item.contextName === oldName);
  return override ? upsertAcceleratorOverride(removeAcceleratorOverride(settings, oldName), { ...override, contextName: newName }) : settings;
}
