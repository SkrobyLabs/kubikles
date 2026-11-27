import { createContext, useContext } from 'react';

/**
 * Whether the bottom-panel tab hosting this content is the one being shown.
 * keepAlive tabs stay mounted behind display:none, so content that does background
 * work (log streaming) reads this to avoid rendering while hidden.
 * Defaults to true for content rendered outside a tab.
 */
export const TabActiveContext = createContext<boolean>(true);

export const useTabActive = () => useContext(TabActiveContext);
