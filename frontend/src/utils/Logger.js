import { LogDebug } from '../../wailsjs/go/main/App';

const formatMessage = (message, data) => {
    if (data) {
        try {
            return `${message} | Data: ${JSON.stringify(data)}`;
        } catch (e) {
            return `${message} | Data: [Circular or Non-Serializable]`;
        }
    }
    return message;
};

const Logger = {
    info: (message, data = null) => {
        const formatted = formatMessage(message, data);
        console.log(`[INFO] ${formatted}`);
        LogDebug(`[INFO] ${formatted}`).catch(err => console.error("Failed to send log to backend:", err));
    },

    error: (message, error = null) => {
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
        LogDebug(`[ERROR] ${formatted}`).catch(err => console.error("Failed to send log to backend:", err));
    },

    debug: (message, data = null) => {
        const formatted = formatMessage(message, data);
        console.debug(`[DEBUG] ${formatted}`);
        // Optional: only send debug logs to backend if verbose mode is on? 
        // For now, let's send them as LogDebug is intended for debug.
        LogDebug(`[DEBUG] ${formatted}`).catch(err => console.error("Failed to send log to backend:", err));
    }
};

export default Logger;
