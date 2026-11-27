import { useEffect, useRef } from 'react';
import { useNotification } from '../context';
import { NOTIFICATION_SOUNDS } from '../components/shared/NotificationSettingsMenu';
import { K8sPod, K8sContainerStatus } from '../types/k8s';
import type { PodTransition } from '../lib/views/protocol';
import type { ViewHandle } from '../lib/views/viewClient';

/**
 * Singleton AudioContext — created once and reused so it works even when
 * the window is not focused. Resuming handles the browser auto-suspend policy.
 */
let _audioCtx: AudioContext | null = null;
function getAudioContext(): AudioContext {
    if (!_audioCtx) {
        _audioCtx = new (window.AudioContext || (window as any).webkitAudioContext)();
    }
    return _audioCtx;
}

/**
 * Warm up the AudioContext on a user gesture (e.g. toggling the bell).
 * Call this from a click handler so the context is allowed to play later.
 */
export function warmUpAudio(): void {
    try {
        const ctx = getAudioContext();
        if (ctx.state === 'suspended') ctx.resume();
    } catch { /* ignore */ }
}

interface NotificationNote {
    freq: number;
    start: number;
}

interface NotificationSound {
    notes: NotificationNote[];
}

/**
 * Plays a notification sound by key.
 */
export function playNotificationSound(soundKey: string = 'alert'): void {
    try {
        const sound = (NOTIFICATION_SOUNDS as Record<string, any>)[soundKey] as NotificationSound | undefined;
        if (!sound || !sound.notes || sound.notes.length === 0) return;

        const ctx = getAudioContext();
        if (ctx.state === 'suspended') ctx.resume();
        const t = ctx.currentTime;

        for (const note of sound.notes) {
            const osc = ctx.createOscillator();
            const gain = ctx.createGain();
            osc.connect(gain);
            gain.connect(ctx.destination);
            osc.frequency.value = note.freq;
            osc.type = 'sine';
            const onset = t + note.start;
            gain.gain.setValueAtTime(0, onset);
            gain.gain.linearRampToValueAtTime(0.25, onset + 0.03);
            gain.gain.exponentialRampToValueAtTime(0.001, onset + 0.5);
            osc.start(onset);
            osc.stop(onset + 0.5);
        }
    } catch {
        // Audio not available
    }
}

/**
 * Gets the owner key from a pod's ownerReferences.
 * Returns "{kind}/{name}" for the first owner, or null if none.
 * @exported for testing
 */
export function getOwnerKey(pod: K8sPod): string | null {
    const owners = pod?.metadata?.ownerReferences;
    if (!owners || owners.length === 0) return null;
    const owner = owners[0];
    return `${owner.kind}/${owner.name}`;
}

/**
 * Gets total restart count across all containers in a pod.
 * @exported for testing
 */
export function getTotalRestarts(pod: K8sPod): number {
    const statuses = pod?.status?.containerStatuses;
    if (!statuses) return 0;
    return statuses.reduce((sum: any, cs: any) => sum + (cs.restartCount || 0), 0);
}

const REDEPLOY_WINDOW_MS = 30000;

interface PodSnapshot {
    total: number;
    containerStatuses: K8sContainerStatus[];
    name: string;
    namespace: string;
    ownerKey: string | null;
}

interface DeletionEntry {
    podName: string;
    namespace: string;
    timestamp: number;
}

interface NotificationSettings {
    soundKey?: string;
    throttleSeconds?: number;
}

interface PendingNotification {
    notification: {
        type: 'info' | 'success' | 'warning' | 'error';
        title: string;
        message: string;
    };
    sound: string;
}

/** A pod event the notification messages are built from, whichever way it was found. */
export type PodEvent =
    | { kind: 'restart'; name: string; namespace: string; total: number; containers: Array<{ name: string; count: number }> }
    | { kind: 'deleted'; name: string; namespace: string; ownerKey: string | null }
    | { kind: 'added'; name: string; ownerKey: string | null };

/**
 * Turns pod events into notifications. Restarts come first, then deletions are remembered per
 * owner, then every new pod whose owner lost a pod within the redeploy window reports a
 * redeploy. The three passes run in that order whatever the order of the events, like the
 * original snapshot diff. Shared by the snapshot diff of the legacy hook and by the transitions
 * a pod view reports.
 */
export function eventsToNotifications(
    events: PodEvent[],
    recentDeletions: Map<string, DeletionEntry>,
    nowFn: () => number = Date.now,
): PendingNotification[] {
    const pending: PendingNotification[] = [];

    for (const ev of events) {
        if (ev.kind !== 'restart') continue;
        const containerInfo = ev.containers.length > 0
            ? ev.containers.map((c: any) => `${c.name} (${c.count})`).join(', ')
            : `total: ${ev.total}`;
        pending.push({
            notification: {
                type: 'warning',
                title: 'Pod Restart',
                message: `${ev.name} in ${ev.namespace} restarted — ${containerInfo}`,
            },
            sound: 'warning',
        });
    }

    for (const ev of events) {
        if (ev.kind !== 'deleted' || !ev.ownerKey) continue;
        recentDeletions.set(ev.ownerKey, { podName: ev.name, namespace: ev.namespace, timestamp: nowFn() });
    }

    for (const ev of events) {
        if (ev.kind !== 'added' || !ev.ownerKey) continue;
        const deletion = recentDeletions.get(ev.ownerKey);
        if (deletion && (nowFn() - deletion.timestamp) < REDEPLOY_WINDOW_MS) {
            const ownerParts = ev.ownerKey.split('/');
            const ownerKind = ownerParts[0];
            const ownerName = ownerParts.slice(1).join('/');
            pending.push({
                notification: {
                    type: 'info',
                    title: 'Pod Redeployed',
                    message: `${ev.name} replaced ${deletion.podName} (${ownerKind}: ${ownerName})`,
                },
                sound: 'info',
            });
            recentDeletions.delete(ev.ownerKey);
        }
    }

    // Clean stale deletion entries
    const now = nowFn();
    for (const [key, entry] of recentDeletions) {
        if (now - entry.timestamp > REDEPLOY_WINDOW_MS * 2) {
            recentDeletions.delete(key);
        }
    }
    return pending;
}

/**
 * Fires notifications outside React's commit phase, with throttling: inside the throttle window
 * the messages are still listed but the sound is skipped.
 */
function dispatchNotifications(
    pending: PendingNotification[],
    settings: NotificationSettings,
    lastNotificationTimeRef: { current: number },
    addNotificationRef: { current: (n: PendingNotification['notification']) => void },
): void {
    if (pending.length === 0) return;
    const { soundKey = 'alert', throttleSeconds = 0 } = settings;
    const now = Date.now();
    const throttleMs = throttleSeconds * 1000;
    const timeSinceLast = now - lastNotificationTimeRef.current;

    // Check if we're within the throttle window
    if (throttleMs > 0 && timeSinceLast < throttleMs) {
        // Still add to notification list, just skip the sound
        setTimeout(() => {
            for (const { notification } of pending) {
                addNotificationRef.current(notification);
            }
        }, 0);
    } else {
        lastNotificationTimeRef.current = now;
        setTimeout(() => {
            for (const { notification } of pending) {
                addNotificationRef.current(notification);
            }
            // Play sound once for batch (using configured sound)
            if (soundKey !== 'none') {
                playNotificationSound(soundKey);
            }
        }, 0);
    }
}

/**
 * Hook that monitors pod changes by comparing snapshots of the pods array.
 * Does NOT subscribe to Wails events directly (avoids EventsOff removing all listeners).
 * Instead, it diffs the current `pods` array against a previous snapshot.
 */
export function usePodNotifications(
    pods: K8sPod[] | null,
    enabled: boolean,
    filteredUids: Set<string> | null = null,
    settings: NotificationSettings = {}
): void {
    const { addNotification } = useNotification();
    const { soundKey = 'alert', throttleSeconds = 0 } = settings;

    // Previous snapshot: Map<uid, PodSnapshot>
    const prevPodsRef = useRef<Map<string, PodSnapshot> | null>(null);
    // Track recently deleted pods for redeploy detection: Map<ownerKey, DeletionEntry>
    const recentDeletionsRef = useRef<Map<string, DeletionEntry>>(new Map());
    // Track last notification time for throttling
    const lastNotificationTimeRef = useRef<number>(0);

    // Store addNotification in a ref so the effect doesn't depend on it
    // (avoids re-running the diff when the context value changes).
    const addNotificationRef = useRef(addNotification);
    useEffect(() => { addNotificationRef.current = addNotification; }, [addNotification]);

    useEffect(() => {
        // While disabled, do no per-batch work at all (no O(n) snapshot). Dropping the snapshot
        // means the first batch after enabling only seeds it, so restarts that happened while
        // disabled are never replayed as a burst of notifications.
        if (!enabled) {
            prevPodsRef.current = null;
            recentDeletionsRef.current.clear();
            return;
        }

        if (!pods || pods.length === 0) {
            // If pods empties out (e.g. namespace switch), don't treat as deletions
            if (pods && pods.length === 0 && prevPodsRef.current && prevPodsRef.current.size > 0) {
                prevPodsRef.current = new Map();
            }
            return;
        }

        // Build current snapshot
        const current = new Map();
        for (const pod of pods) {
            const uid = pod?.metadata?.uid;
            if (!uid) continue;
            current.set(uid, {
                total: getTotalRestarts(pod),
                containerStatuses: pod?.status?.containerStatuses || [],
                name: pod.metadata?.name || 'unknown',
                namespace: pod.metadata?.namespace || '',
                ownerKey: getOwnerKey(pod),
            });
        }

        const prev = prevPodsRef.current;

        // First time: just seed, don't notify
        if (!prev) {
            prevPodsRef.current = current;
            return;
        }

        const isVisible = (uid: string): boolean => !filteredUids || filteredUids.has(uid);

        // Collect the events of this batch; notifications are built and fired outside the render cycle
        const events: PodEvent[] = [];

        // --- Detect restarts (pods present in both prev and current with higher restartCount) ---
        for (const [uid, curr] of current) {
            const old = prev.get(uid);
            if (!old) continue; // new pod — handled below
            if (curr.total > old.total && isVisible(uid)) {
                // Find which containers restarted
                const oldMap: Record<string, number> = {};
                for (const cs of old.containerStatuses) {
                    oldMap[cs.name] = cs.restartCount || 0;
                }
                const restarted: Array<{ name: string; count: number }> = [];
                for (const cs of curr.containerStatuses) {
                    const oldCount = oldMap[cs.name] ?? 0;
                    if ((cs.restartCount || 0) > oldCount) {
                        restarted.push({ name: cs.name, count: cs.restartCount });
                    }
                }
                events.push({ kind: 'restart', name: curr.name, namespace: curr.namespace, total: curr.total, containers: restarted });
            }
        }

        // --- Detect deletions (pods in prev but not in current) ---
        for (const [uid, old] of prev) {
            if (!current.has(uid) && isVisible(uid)) {
                events.push({ kind: 'deleted', name: old.name, namespace: old.namespace, ownerKey: old.ownerKey });
            }
        }

        // --- Detect redeploys (new pods whose owner had a recent deletion) ---
        for (const [uid, curr] of current) {
            if (prev.has(uid)) continue; // not new
            events.push({ kind: 'added', name: curr.name, ownerKey: curr.ownerKey });
        }

        const pending = eventsToNotifications(events, recentDeletionsRef.current);

        prevPodsRef.current = current;

        dispatchNotifications(pending, { soundKey, throttleSeconds }, lastNotificationTimeRef, addNotificationRef);
    }, [pods, enabled, filteredUids, soundKey, throttleSeconds]);
}

/** The pod events of the transitions a pod view reports (notifyOn "pods") in the shape the messages are built from. */
export function eventsFromTransitions(transitions: PodTransition[]): PodEvent[] {
    const out: PodEvent[] = [];
    for (const t of transitions) {
        if (t.kind === 'restart') {
            out.push({ kind: 'restart', name: t.name, namespace: t.namespace, total: t.total ?? 0, containers: t.containers ?? [] });
        } else if (t.kind === 'deleted') {
            out.push({ kind: 'deleted', name: t.name, namespace: t.namespace, ownerKey: t.owner || null });
        } else if (t.kind === 'added') {
            out.push({ kind: 'added', name: t.name, ownerKey: t.owner || null });
        }
    }
    return out;
}

/**
 * Pod notifications for a pod list served by a Go view. The view opened with notifyOn "pods"
 * finds the restarts, deletions and new pods itself (restarts and deletions of pods the list's
 * search and filters show), so the whole pod list never reaches the page; this hook turns what
 * the view reports into the same messages, redeploy detection, throttling and sound as
 * usePodNotifications.
 */
export function usePodViewNotifications(
    view: Pick<ViewHandle, 'onTransitions'> | null,
    enabled: boolean,
    settings: NotificationSettings = {},
): void {
    const { addNotification } = useNotification();
    const recentDeletionsRef = useRef<Map<string, DeletionEntry>>(new Map());
    const lastNotificationTimeRef = useRef<number>(0);
    const addNotificationRef = useRef(addNotification);
    useEffect(() => { addNotificationRef.current = addNotification; }, [addNotification]);
    const settingsRef = useRef(settings);
    settingsRef.current = settings;

    useEffect(() => {
        const deletions = recentDeletionsRef.current;
        if (!view || !enabled) {
            deletions.clear();
            return;
        }
        const off = view.onTransitions(transitions => {
            const pending = eventsToNotifications(eventsFromTransitions(transitions), deletions);
            dispatchNotifications(pending, settingsRef.current, lastNotificationTimeRef, addNotificationRef);
        });
        return () => { off(); deletions.clear(); };
    }, [view, enabled]);
}
