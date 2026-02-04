import { useEffect, useRef } from 'react';
import { useNotification } from '../context';
import { NOTIFICATION_SOUNDS } from '../components/shared/NotificationSettingsMenu';

/**
 * Singleton AudioContext — created once and reused so it works even when
 * the window is not focused. Resuming handles the browser auto-suspend policy.
 */
let _audioCtx = null;
function getAudioContext() {
    if (!_audioCtx) {
        _audioCtx = new (window.AudioContext || window.webkitAudioContext)();
    }
    return _audioCtx;
}

/**
 * Warm up the AudioContext on a user gesture (e.g. toggling the bell).
 * Call this from a click handler so the context is allowed to play later.
 */
export function warmUpAudio() {
    try {
        const ctx = getAudioContext();
        if (ctx.state === 'suspended') ctx.resume();
    } catch { /* ignore */ }
}

/**
 * Plays a notification sound by key.
 * @param {string} soundKey - Key from NOTIFICATION_SOUNDS (e.g., 'chime', 'alert', 'ding')
 */
export function playNotificationSound(soundKey = 'alert') {
    try {
        const sound = NOTIFICATION_SOUNDS[soundKey];
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
export function getOwnerKey(pod) {
    const owners = pod?.metadata?.ownerReferences;
    if (!owners || owners.length === 0) return null;
    const owner = owners[0];
    return `${owner.kind}/${owner.name}`;
}

/**
 * Gets total restart count across all containers in a pod.
 * @exported for testing
 */
export function getTotalRestarts(pod) {
    const statuses = pod?.status?.containerStatuses;
    if (!statuses) return 0;
    return statuses.reduce((sum, cs) => sum + (cs.restartCount || 0), 0);
}

const REDEPLOY_WINDOW_MS = 30000;

/**
 * Hook that monitors pod changes by comparing snapshots of the pods array.
 * Does NOT subscribe to Wails events directly (avoids EventsOff removing all listeners).
 * Instead, it diffs the current `pods` array against a previous snapshot.
 *
 * @param {Array} pods - Current pods array (updated by useResourceWatcher)
 * @param {boolean} enabled - Whether notifications are enabled
 * @param {Set<string>|null} filteredUids - Set of visible pod UIDs (null = all pods)
 * @param {object} settings - Notification settings
 * @param {string} settings.soundKey - Sound key from NOTIFICATION_SOUNDS
 * @param {number} settings.throttleSeconds - Minimum seconds between notifications (0 = no throttle)
 */
export function usePodNotifications(pods, enabled, filteredUids = null, settings = {}) {
    const { addNotification } = useNotification();
    const { soundKey = 'alert', throttleSeconds = 0 } = settings;

    // Previous snapshot: Map<uid, { total, containerStatuses, name, namespace, ownerKey }>
    const prevPodsRef = useRef(null);
    // Track recently deleted pods for redeploy detection: Map<ownerKey, { podName, namespace, timestamp }>
    const recentDeletionsRef = useRef(new Map());
    // Track last notification time for throttling
    const lastNotificationTimeRef = useRef(0);

    // Store addNotification in a ref so the effect doesn't depend on it
    // (avoids re-running the diff when the context value changes).
    const addNotificationRef = useRef(addNotification);
    useEffect(() => { addNotificationRef.current = addNotification; }, [addNotification]);

    useEffect(() => {
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

        if (!enabled) {
            prevPodsRef.current = current;
            return;
        }

        const isVisible = (uid) => !filteredUids || filteredUids.has(uid);

        // Collect notifications to fire outside the render cycle
        const pending = [];

        // --- Detect restarts (pods present in both prev and current with higher restartCount) ---
        for (const [uid, curr] of current) {
            const old = prev.get(uid);
            if (!old) continue; // new pod — handled below
            if (curr.total > old.total && isVisible(uid)) {
                // Find which containers restarted
                const oldMap = {};
                for (const cs of old.containerStatuses) {
                    oldMap[cs.name] = cs.restartCount || 0;
                }
                const restarted = [];
                for (const cs of curr.containerStatuses) {
                    const oldCount = oldMap[cs.name] ?? 0;
                    if ((cs.restartCount || 0) > oldCount) {
                        restarted.push({ name: cs.name, count: cs.restartCount });
                    }
                }

                const containerInfo = restarted.length > 0
                    ? restarted.map(c => `${c.name} (${c.count})`).join(', ')
                    : `total: ${curr.total}`;

                pending.push({
                    notification: {
                        type: 'warning',
                        title: 'Pod Restart',
                        message: `${curr.name} in ${curr.namespace} restarted — ${containerInfo}`,
                    },
                    sound: 'warning',
                });
            }
        }

        // --- Detect deletions (pods in prev but not in current) ---
        for (const [uid, old] of prev) {
            if (!current.has(uid) && isVisible(uid)) {
                if (old.ownerKey) {
                    recentDeletionsRef.current.set(old.ownerKey, {
                        podName: old.name,
                        namespace: old.namespace,
                        timestamp: Date.now(),
                    });
                }
            }
        }

        // --- Detect redeploys (new pods whose owner had a recent deletion) ---
        for (const [uid, curr] of current) {
            if (prev.has(uid)) continue; // not new
            if (!curr.ownerKey) continue;

            const deletion = recentDeletionsRef.current.get(curr.ownerKey);
            if (deletion && (Date.now() - deletion.timestamp) < REDEPLOY_WINDOW_MS) {
                const ownerParts = curr.ownerKey.split('/');
                const ownerKind = ownerParts[0];
                const ownerName = ownerParts.slice(1).join('/');

                pending.push({
                    notification: {
                        type: 'info',
                        title: 'Pod Redeployed',
                        message: `${curr.name} replaced ${deletion.podName} (${ownerKind}: ${ownerName})`,
                    },
                    sound: 'info',
                });
                recentDeletionsRef.current.delete(curr.ownerKey);
            }
        }

        // Clean stale deletion entries
        const now = Date.now();
        for (const [key, entry] of recentDeletionsRef.current) {
            if (now - entry.timestamp > REDEPLOY_WINDOW_MS * 2) {
                recentDeletionsRef.current.delete(key);
            }
        }

        prevPodsRef.current = current;

        // Fire notifications outside React's commit phase (with throttling)
        if (pending.length > 0) {
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
    }, [pods, enabled, filteredUids, soundKey, throttleSeconds]);
}
