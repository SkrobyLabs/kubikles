import React, { createContext, useContext, useState, useCallback } from 'react';

const DebugContext = createContext();

export const useDebug = () => {
    const context = useContext(DebugContext);
    if (!context) {
        throw new Error('useDebug must be used within a DebugProvider');
    }
    return context;
};

export const DebugProvider = ({ children }) => {
    const [isDebugMode, setIsDebugMode] = useState(false);

    const toggleDebugMode = useCallback(() => {
        setIsDebugMode(prev => !prev);
    }, []);

    const enableDebugMode = useCallback(() => {
        setIsDebugMode(true);
    }, []);

    const disableDebugMode = useCallback(() => {
        setIsDebugMode(false);
    }, []);

    const value = {
        isDebugMode,
        toggleDebugMode,
        enableDebugMode,
        disableDebugMode
    };

    return (
        <DebugContext.Provider value={value}>
            {children}
        </DebugContext.Provider>
    );
};
