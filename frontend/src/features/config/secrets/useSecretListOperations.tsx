import { useEffect, useMemo, useRef, useState } from 'react';
import { useK8s } from '~/context';
import { optimizeNamespaceQuery } from '~/hooks/useNamespaceOptimization';
import type { SecretReadSource } from './secretReadSource';
import { normalizeSecretNamespaces, SecretListOperationController, type SecretListState } from './secretListOperations';

const normalizedQuery = (selectedNamespaces: string | string[], allNamespaces: string[]) => {
  const optimized = optimizeNamespaceQuery(selectedNamespaces, allNamespaces);
  if (optimized === null) return [];
  if (optimized === '') return [''];
  return normalizeSecretNamespaces(optimized, allNamespaces);
};

export function useSecretListOperations(
  source: SecretReadSource,
  currentContext: string,
  selectedNamespaces: string | string[],
  allNamespaces: string[],
  visible: boolean,
  hideHelm: boolean,
) {
  const { lastRefresh, reconcileToken, checkConnectionError } = useK8s();
  const [state, setState] = useState<SecretListState>({ secrets: [], loading: false, error: null, loadingProgress: null });
  const checkConnectionErrorRef = useRef(checkConnectionError);
  checkConnectionErrorRef.current = checkConnectionError;
  const controller = useMemo(
    () => new SecretListOperationController(source, setState, error => checkConnectionErrorRef.current(error)),
    [],
  );
  const namespaces = normalizedQuery(selectedNamespaces, allNamespaces);
  const namespaceKey = JSON.stringify(namespaces);
  const refreshRef = useRef({ lastRefresh, reconcileToken });

  useEffect(() => {
    void controller.replace(source, namespaces, hideHelm, Boolean(visible && currentContext));
  }, [controller, source, currentContext, namespaceKey, visible, hideHelm]);

  useEffect(() => {
    const previous = refreshRef.current;
    refreshRef.current = { lastRefresh, reconcileToken };
    if (previous.lastRefresh !== lastRefresh || previous.reconcileToken !== reconcileToken) controller.reconcile();
  }, [controller, lastRefresh, reconcileToken]);

  useEffect(() => () => { void controller.stop(); }, [controller]);

  return state;
}
