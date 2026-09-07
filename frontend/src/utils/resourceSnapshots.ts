import { observeNamespaceList } from './namespaceQuery';

// Short-lived, bounded navigation snapshots. Every reuse still performs a LIST.
const MAX_BYTES = 16 * 1024 * 1024;
const TTL_MS = 30_000;
const snapshots = new Map<string, { map: Map<string, any>; bytes: number; expires: number }>();

export function readResourceSnapshot<T>(key: string): Map<string, T> | undefined {
    const entry = snapshots.get(key);
    if (!entry) return;
    if (entry.expires <= Date.now()) { snapshots.delete(key); return; }
    snapshots.delete(key);
    snapshots.set(key, entry);
    return entry.map;
}

export function writeResourceSnapshot<T>(key: string, map: Map<string, T>): void {
    snapshots.delete(key);
    const bytes = observeNamespaceList(Array.from(map.values()), 0).bytes;
    if (bytes > MAX_BYTES || map.size > 10000) return;
    snapshots.set(key, { map, bytes, expires: Date.now() + TTL_MS });
    let total = 0;
    for (const [key, entry] of snapshots) {
        if (entry.expires <= Date.now()) snapshots.delete(key);
        else total += entry.bytes;
    }
    while (snapshots.size > 8 || total > MAX_BYTES) {
        const oldest = snapshots.keys().next().value!;
        total -= snapshots.get(oldest)!.bytes;
        snapshots.delete(oldest);
    }
}

export function deleteResourceSnapshot(key: string): void { snapshots.delete(key); }
