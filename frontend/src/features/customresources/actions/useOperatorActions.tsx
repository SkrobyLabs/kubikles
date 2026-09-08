import React, { useEffect, useState } from 'react';
import { GetResourceActions } from 'wailsjs/go/main/App';
import type { CRDInfo } from '../instances/useCustomResourceActions';
import type { OperatorAction, ResourceRef } from './types';
import OperatorActionDialog from './OperatorActionDialog';

export function useOperatorActions(crdInfo: CRDInfo, context: string) {
    const [actions, setActions] = useState<OperatorAction[]>([]);
    const [active, setActive] = useState<{ action: OperatorAction; ref: ResourceRef } | null>(null);
    useEffect(() => {
        let cancelled = false;
        setActions([]);
        GetResourceActions(crdInfo.group, crdInfo.resource).then((result: OperatorAction[]) => {
            if (!cancelled) setActions(result || []);
        }).catch((err: unknown) => console.error('Failed to load operator actions', err));
        return () => { cancelled = true; };
    }, [crdInfo.group, crdInfo.resource]);
    const openAction = (action: OperatorAction, resource: any) => setActive({ action, ref: {
        context, group: crdInfo.group, version: crdInfo.version, resource: crdInfo.resource,
        namespace: resource.metadata?.namespace || '', name: resource.metadata?.name, uid: resource.metadata?.uid,
    } });
    return { actions, openAction, dialog: active && <OperatorActionDialog
        action={active.action} source={active.ref} onClose={() => setActive(null)}
    /> };
}
