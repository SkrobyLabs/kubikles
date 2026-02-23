import React, { createContext, useContext, useState, useCallback, useMemo } from 'react';

type NotificationType = 'info' | 'success' | 'warning' | 'error';

interface Notification {
    id: number;
    type: NotificationType;
    title?: string;
    message: string;
    createdAt: number;
}

interface AddNotificationParams {
    type?: NotificationType;
    title?: string;
    message: string;
    duration?: number;
}

interface NotificationContextValue {
    notifications: Notification[];
    addNotification: (params: AddNotificationParams) => number;
    removeNotification: (id: number) => void;
}

const NotificationContext = createContext<NotificationContextValue | undefined>(undefined);

let notificationId = 0;

export const NotificationProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [notifications, setNotifications] = useState<Notification[]>([]);

    const addNotification = useCallback(({ type = 'info', title, message, duration = 8000 }: AddNotificationParams): number => {
        const id = ++notificationId;
        const notification: Notification = {
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

    const removeNotification = useCallback((id: number): void => {
        setNotifications(prev => prev.filter((n: any) => n.id !== id));
    }, []);

    const contextValue: NotificationContextValue = useMemo(() => ({
        notifications,
        addNotification,
        removeNotification,
    }), [notifications, addNotification, removeNotification]);

    return (
        <NotificationContext.Provider value={contextValue}>
            {children}
        </NotificationContext.Provider>
    );
};

export const useNotification = (): NotificationContextValue => {
    const context = useContext(NotificationContext);
    if (!context) {
        throw new Error('useNotification must be used within a NotificationProvider');
    }
    return context;
};
