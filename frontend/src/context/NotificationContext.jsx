import React, { createContext, useContext, useState, useCallback, useMemo } from 'react';

const NotificationContext = createContext(null);

let notificationId = 0;

export function NotificationProvider({ children }) {
    const [notifications, setNotifications] = useState([]);

    const addNotification = useCallback(({ type = 'info', title, message, duration = 8000 }) => {
        const id = ++notificationId;
        const notification = {
            id,
            type,
            title,
            message,
            createdAt: Date.now()
        };

        setNotifications(prev => [...prev, notification]);

        // Auto-dismiss after duration (0 = never auto-dismiss)
        if (duration > 0) {
            setTimeout(() => {
                removeNotification(id);
            }, duration);
        }

        return id;
    }, []);

    const removeNotification = useCallback((id) => {
        setNotifications(prev => prev.filter(n => n.id !== id));
    }, []);

    const clearAll = useCallback(() => {
        setNotifications([]);
    }, []);

    const contextValue = useMemo(() => ({
        notifications,
        addNotification,
        removeNotification,
        clearAll
    }), [notifications, addNotification, removeNotification, clearAll]);

    return (
        <NotificationContext.Provider value={contextValue}>
            {children}
        </NotificationContext.Provider>
    );
}

export function useNotification() {
    const context = useContext(NotificationContext);
    if (!context) {
        throw new Error('useNotification must be used within a NotificationProvider');
    }
    return context;
}
