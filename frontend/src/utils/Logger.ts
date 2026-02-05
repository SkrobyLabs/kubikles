import { LogMessage } from '../../wailsjs/go/main/App';

const formatMessage = (message: string, data: unknown): string => {
    if (data) {
        try {
            return `${message} | Data: ${JSON.stringify(data)}`;
        } catch (e) {
            return `${message} | Data: [Circular or Non-Serializable]`;
        }
    }
    return message;
};

interface Logger {
    info: (message: string, data?: unknown) => void;
    error: (message: string, error?: Error | unknown) => void;
    debug: (message: string, data?: unknown) => void;
    warn: (message: string, data?: unknown) => void;
}

const Logger: Logger = {
    info: (message: string, data: unknown = null) => {
        const formatted = formatMessage(message, data);
        console.log(`[INFO] ${formatted}`);
        LogMessage(`[INFO] ${formatted}`).catch(err => console.error("Failed to send log to backend:", err));
    },

    error: (message: string, error: Error | unknown = null) => {
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
        LogMessage(`[ERROR] ${formatted}`).catch(err => console.error("Failed to send log to backend:", err));
    },

    debug: (message: string, data: unknown = null) => {
        const formatted = formatMessage(message, data);
        console.debug(`[DEBUG] ${formatted}`);
        // Optional: only send debug logs to backend if verbose mode is on?
        // For now, let's send them as LogMessage is intended for debug.
        LogMessage(`[DEBUG] ${formatted}`).catch(err => console.error("Failed to send log to backend:", err));
    },

    warn: (message: string, data: unknown = null) => {
        const formatted = formatMessage(message, data);
        console.warn(`[WARN] ${formatted}`);
        LogMessage(`[WARN] ${formatted}`).catch(err => console.error("Failed to send log to backend:", err));
    }
};

export default Logger;
