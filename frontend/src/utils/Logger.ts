import type { DebugCategory } from '../context/DebugContext';

const formatMessage = (message: string, data: unknown): string => {
    if (data) {
        try {
            return `${message} | Data: ${JSON.stringify(data)}`;
        } catch (e: any) {
            return `${message} | Data: [Circular or Non-Serializable]`;
        }
    }
    return message;
};

const dispatchDebugEvent = (level: string, message: string, data: unknown, category: DebugCategory): void => {
    const details = data != null ? { level, data } : { level };
    window.dispatchEvent(new CustomEvent('frontend-debug-log', {
        detail: { category, message, details }
    }));
};

interface Logger {
    info: (message: string, data?: unknown, category?: DebugCategory) => void;
    error: (message: string, error?: Error | unknown, category?: DebugCategory) => void;
    debug: (message: string, data?: unknown, category?: DebugCategory) => void;
    warn: (message: string, data?: unknown, category?: DebugCategory) => void;
}

const Logger: Logger = {
    info: (message: string, data: unknown = null, category: DebugCategory = 'ui') => {
        const formatted = formatMessage(message, data);
        console.log(`[INFO] ${formatted}`);
        dispatchDebugEvent('INFO', message, data, category);
    },

    error: (message: string, error: Error | unknown = null, category: DebugCategory = 'ui') => {
        let errorMsg = '';
        if (error) {
            if (error instanceof Error) {
                errorMsg = ` | Error: ${error.message}\nStack: ${error.stack}`;
            } else {
                errorMsg = ` | Error: ${JSON.stringify(error)}`;
            }
        }
        const formatted = `${message}${errorMsg}`;
        console.error(`[ERROR] ${formatted}`);
        dispatchDebugEvent('ERROR', message, error, category);
    },

    debug: (message: string, data: unknown = null, category: DebugCategory = 'ui') => {
        const formatted = formatMessage(message, data);
        console.debug(`[DEBUG] ${formatted}`);
        dispatchDebugEvent('DEBUG', message, data, category);
    },

    warn: (message: string, data: unknown = null, category: DebugCategory = 'ui') => {
        const formatted = formatMessage(message, data);
        console.warn(`[WARN] ${formatted}`);
        dispatchDebugEvent('WARN', message, data, category);
    }
};

export default Logger;
