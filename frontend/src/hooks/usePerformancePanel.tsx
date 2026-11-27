import { useCallback, useRef } from 'react';
import { useUI } from '../context';
import PerformancePanel from '../components/shared/PerformancePanel';

const TAB_ID = 'performance-panel';

/**
 * Opens the Performance tab. Polling state lives inside the panel component
 * (mounted only while its tab is active), so this hook holds no state that
 * would re-render App or UIContext subscribers on every metrics tick.
 */
export const usePerformancePanel = (): {
    openPerformancePanel: () => void;
} => {
    const { openTab, bottomTabs } = useUI();
    const bottomTabsRef = useRef(bottomTabs);
    bottomTabsRef.current = bottomTabs;

    const openPerformancePanel = useCallback((): void => {
        const existingTab = bottomTabsRef.current.find((t: any) => t.id === TAB_ID);
        if (existingTab) {
            openTab(existingTab);
            return;
        }
        openTab({
            id: TAB_ID,
            title: 'Performance',
            context: null, // Context-independent
            content: <PerformancePanel />,
        });
    }, [openTab]);

    return { openPerformancePanel };
};
