import { useEffect, useImperativeHandle, useMemo, useState, forwardRef } from 'react';
import type { SecretReadSource } from '../features/config/secrets/secretReadSourceContract';
import type { AcceleratorSecretDataEntry, AcceleratorSecretSummary } from './facade';

export type BrowserSecretDetailHandle = { clearSensitiveState(): void };

type Props = {
  source: SecretReadSource;
  secret: AcceleratorSecretSummary;
  onBack(): void;
};

const BrowserSecretDetail = forwardRef<BrowserSecretDetailHandle, Props>(function BrowserSecretDetail({ source, secret, onBack }, ref) {
  const [tab, setTab] = useState<'yaml' | 'values'>('values');
  const [yaml, setYaml] = useState('');
  const [entries, setEntries] = useState<AcceleratorSecretDataEntry[]>([]);
  const [showBase64, setShowBase64] = useState(true);
  const [filter, setFilter] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(false);

  const clear = () => {
    setYaml('');
    setEntries([]);
    setFilter('');
    setError(false);
    setLoading(false);
  };
  useImperativeHandle(ref, () => ({ clearSensitiveState: clear }));

  useEffect(() => {
    let active = true;
    setYaml('');
    setEntries([]);
    setFilter('');
    setError(false);
    setLoading(true);
    Promise.all([
      source.getSecretYaml(secret.metadata.namespace, secret.metadata.name),
      source.getSecretData(secret.metadata.namespace, secret.metadata.name),
    ]).then(([nextYaml, nextEntries]) => {
      if (!active) return;
      setYaml(typeof nextYaml === 'string' ? nextYaml : '');
      setEntries(Array.isArray(nextEntries) ? nextEntries : []);
    }).catch(() => {
      if (active) setError(true);
    }).finally(() => {
      if (active) setLoading(false);
    });
    return () => {
      active = false;
      setYaml('');
      setEntries([]);
    };
  }, [source, secret.metadata.uid, secret.metadata.namespace, secret.metadata.name]);

  const visibleEntries = useMemo(() => entries.filter(entry => entry.key.toLocaleLowerCase().includes(filter.toLocaleLowerCase())), [entries, filter]);
  return <section className="browser-panel browser-detail" aria-label="Secret detail">
    <div className="browser-detail-header"><button type="button" onClick={() => { clear(); onBack(); }}>Back</button><h2>{secret.metadata.namespace} / {secret.metadata.name}</h2></div>
    <div className="browser-tabs"><button type="button" aria-pressed={tab === 'yaml'} onClick={() => setTab('yaml')}>YAML</button><button type="button" aria-pressed={tab === 'values'} onClick={() => setTab('values')}>Values</button></div>
    {loading && <div className="browser-state">Loading Secret detail</div>}
    {!loading && error && <div className="browser-state browser-error">Unable to load Secret detail</div>}
    {!loading && !error && tab === 'yaml' && <pre>{yaml}</pre>}
    {!loading && !error && tab === 'values' && <div>
      <div className="browser-toolbar">
        <label>Filter keys<input aria-label="Filter keys" value={filter} onChange={event => setFilter(event.target.value)} /></label>
        <button type="button" aria-pressed={showBase64} onClick={() => setShowBase64(true)}>Base64</button>
        <button type="button" aria-pressed={!showBase64} onClick={() => setShowBase64(false)}>Decoded</button>
      </div>
      {visibleEntries.length === 0 ? <div className="browser-state">No values found</div> : <dl className="browser-values">{visibleEntries.map(entry => <div key={entry.key}>
        <dt>{entry.key}{(entry.isBinary || entry.encoding === 'base64') && <span> binary</span>}</dt>
        <dd><pre>{showBase64 || entry.isBinary || entry.encoding === 'base64' ? entry.base64Value : entry.value}</pre></dd>
      </div>)}</dl>}
    </div>}
  </section>;
});

export default BrowserSecretDetail;
