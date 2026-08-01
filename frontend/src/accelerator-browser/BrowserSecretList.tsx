import { useEffect, useMemo, useState } from 'react';
import { SecretListOperationController } from '../features/config/secrets/secretListOperations';
import type { SecretListState, SecretReadSource } from '../features/config/secrets/secretReadSourceContract';
import { formatAge } from '../utils/formatting';
import type { AcceleratorSecretSummary } from './facade';

type Props = {
  visible: boolean;
  source: SecretReadSource;
  selectedNamespace: string;
  hideHelm: boolean;
  search: string;
  onNamespacesObserved(namespaces: string[]): void;
  onSelect(secret: AcceleratorSecretSummary): void;
  registerStop(stop: () => Promise<void>): () => void;
};

const sortRows = (left: AcceleratorSecretSummary, right: AcceleratorSecretSummary) => {
  const time = Date.parse(right.metadata.creationTimestamp) - Date.parse(left.metadata.creationTimestamp);
  return time || left.metadata.uid.localeCompare(right.metadata.uid);
};

export default function BrowserSecretList({ visible, source, selectedNamespace, hideHelm, search, onNamespacesObserved, onSelect, registerStop }: Props) {
  const [state, setState] = useState<SecretListState>({ secrets: [], loading: false, error: null, loadingProgress: null });
  const [notice, setNotice] = useState(false);
  const controller = useMemo(() => new SecretListOperationController(source, setState, () => false, () => setNotice(true)), [source]);
  const normalizedNamespaces = selectedNamespace === '*' ? [''] : [selectedNamespace];
  const namespaceKey = JSON.stringify(normalizedNamespaces);

  useEffect(() => {
    setNotice(false);
    void controller.replace(source, normalizedNamespaces, hideHelm, true);
  }, [controller, source, namespaceKey, hideHelm]);
  useEffect(() => registerStop(() => controller.stop()), [controller, registerStop]);

  const projectedRows = useMemo(() => (state.secrets as AcceleratorSecretSummary[]), [state.secrets]);
  const observedKey = projectedRows.map(row => row.metadata.namespace).sort().join('\u0000');
  useEffect(() => {
    onNamespacesObserved(projectedRows.map(row => row.metadata.namespace));
  }, [observedKey, onNamespacesObserved]);

  const query = search.toLocaleLowerCase();
  const rows = projectedRows.filter(row => !query ||
    row.metadata.name.toLocaleLowerCase().includes(query) ||
    row.metadata.namespace.toLocaleLowerCase().includes(query) ||
    row.type.toLocaleLowerCase().includes(query))
    .sort(sortRows);

  if (!visible) return null;
  return <section className="browser-panel" aria-label="Secret list">
    {notice && <div className="browser-state browser-error">Some Secrets could not be loaded</div>}
    {state.loading && <div className="browser-state">{state.loadingProgress ? `Loading Secrets ${state.loadingProgress.loaded} of ${state.loadingProgress.total}` : 'Loading Secrets'}</div>}
    {!state.loading && state.error && <div className="browser-state browser-error">Unable to load Secrets</div>}
    {!state.loading && !state.error && rows.length === 0 && <div className="browser-state">No Secrets found</div>}
    {rows.length > 0 && <table>
      <thead><tr><th>Name</th><th>Namespace</th><th>Type</th><th>Age</th><th>Keys</th></tr></thead>
      <tbody>{rows.map(row => <tr key={row.metadata.uid} tabIndex={0} onClick={() => onSelect(row)} onKeyDown={event => {
        if (event.key === 'Enter' || event.key === ' ') onSelect(row);
      }}>
        <td><button type="button" className="browser-row-link" aria-label={`Open ${row.metadata.name}`} onClick={event => { event.stopPropagation(); onSelect(row); }}>{row.metadata.name}</button></td><td>{row.metadata.namespace}</td><td>{row.type}</td>
        <td>{formatAge(row.metadata.creationTimestamp)}</td>
        <td>{row.dataKeys === 0 ? '-' : `${row.dataKeys} key${row.dataKeys === 1 ? '' : 's'}`}</td>
      </tr>)}</tbody>
    </table>}
  </section>;
}
