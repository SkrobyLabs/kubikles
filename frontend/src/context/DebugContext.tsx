import React, { createContext, useContext, useState, useCallback, useMemo } from 'react';

interface DebugContextValue {
    isDebugMode: boolean;
    toggleDebugMode: () => void;
    enableDebugMode: () => void;
    disableDebugMode: () => void;
}

const DebugContext = createContext<DebugContextValue | undefined>(undefined);

export const useDebug = (): DebugContextValue => {
    const context = useContext(DebugContext);
    if (!context) {
        throw new Error('useDebug must be used within a DebugProvider');
    }
    return context;
};

export const DebugProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [isDebugMode, setIsDebugMode] = useState<boolean>(false);

    const toggleDebugMode = useCallback((): void => {
        setIsDebugMode(prev => !prev);
    }, []);

    const enableDebugMode = useCallback((): void => {
        setIsDebugMode(true);
    }, []);

    const disableDebugMode = useCallback((): void => {
        setIsDebugMode(false);
    }, []);

    const value: DebugContextValue = useMemo(() => ({
        isDebugMode,
        toggleDebugMode,
        enableDebugMode,
        disableDebugMode
    }), [isDebugMode, toggleDebugMode, enableDebugMode, disableDebugMode]);

    return (
        <DebugContext.Provider value={value}>
            {children}
        </DebugContext.Provider>
    );
};
