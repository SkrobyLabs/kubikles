import { useUI } from '../context';
import { useDebug } from '../context';
import DebugLogViewer from '../components/shared/DebugLogViewer';

export const useDebugLogs = (): {
    toggleDebug: () => void;
} => {
    const { openTab, bottomTabs } = useUI();
    const { enableDebugMode } = useDebug();

    const toggleDebug = (): void => {
        const debugTabId = 'debug-logs';
        const existingTab = bottomTabs.find((t: any) => t.id === debugTabId);

        enableDebugMode();

        if (!existingTab) {
            openTab({
                id: debugTabId,
                title: 'Debug Logs',
                context: null,
                content: <DebugLogViewer />
            });
        } else {
            openTab(existingTab);
        }
    };

    return { toggleDebug };
};
