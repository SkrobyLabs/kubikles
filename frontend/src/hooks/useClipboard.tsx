import { useState, useCallback } from 'react';

interface UseClipboardReturn {
  copied: boolean;
  copy: (text: string) => Promise<void>;
  reset: () => void;
}

/**
 * Hook for clipboard operations with copy status tracking
 * @param timeout - Duration in ms to show copied status (default: 2000)
 * @returns Object with copied state, copy function, and reset function
 */
export const useClipboard = (timeout: number = 2000): UseClipboardReturn => {
  const [copied, setCopied] = useState<boolean>(false);

  const copy = useCallback(async (text: string): Promise<void> => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), timeout);
    } catch (error: any) {
      console.error('Failed to copy to clipboard:', error);
      throw error;
    }
  }, [timeout]);

  const reset = useCallback((): void => {
    setCopied(false);
  }, []);

  return { copied, copy, reset };
};
