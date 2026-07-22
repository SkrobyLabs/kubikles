export type ConnectionMode = 'streaming' | 'polling';

export const restoreConnectionMode = (context: string, storage: Pick<Storage, 'getItem'> = localStorage): ConnectionMode => (context && storage.getItem(`kubikles_connection_mode_${context}`) === 'polling' ? 'polling' : 'streaming');

export const isStreamTransportError = (error: unknown): boolean => /watch|stream|upgrade|method not allowed|status code 405|unexpected eof|context deadline exceeded|awaiting headers|connection reset|http2.*closed|proxy.*closed/i.test(String(error));

export const isImmediateWatchClosure = (event: { premature?: boolean; receivedAny?: boolean }): boolean => event.premature === true && event.receivedAny !== true;

export const streamingWarningDismissalKey = (context: string): string => `kubikles_streaming_warning_dismissed_${context}`;

export const isStreamingWarningDismissed = (context: string, storage: Pick<Storage, 'getItem'> = localStorage): boolean => Boolean(context) && storage.getItem(streamingWarningDismissalKey(context)) === 'true';
