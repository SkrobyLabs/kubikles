import type { AcceleratorConnectionOverride } from '~/context/ConfigContext';

export type AcceleratorSettings = { enabledByDefault: boolean; defaultNamespace: string; connectionOverrides: AcceleratorConnectionOverride[] };
export function acceleratorPolicy(settings: AcceleratorSettings, contextName: string, contextNamespace = 'default') {
  const override = settings.connectionOverrides.find(item => item.contextName === contextName);
  const inheritedNamespace = contextNamespace === '*' ? '' : contextNamespace;
  const overrideNamespace = override?.namespace;
  const usesContextNamespace = overrideNamespace === '' || overrideNamespace === '*' || (overrideNamespace === undefined && (!settings.defaultNamespace || settings.defaultNamespace === '*'));
  const namespace = usesContextNamespace ? inheritedNamespace : (overrideNamespace ?? settings.defaultNamespace);
  return {
    enabled: override?.enabled ?? settings.enabledByDefault,
    namespace,
    namespaceSource: usesContextNamespace ? 'context namespace' : (overrideNamespace !== undefined ? 'connection override' : 'Kubikles Settings'),
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
