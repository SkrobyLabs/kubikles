import React from 'react'
import { createRoot } from 'react-dom/client'
import loader from '@monaco-editor/loader'
import { WindowToggleMaximise } from '../wailsjs/runtime/runtime'
import './index.css'
import App from './App'
import ErrorBoundary from './components/shared/ErrorBoundary'

// Configure Monaco to load from local files instead of CDN
// This is required for Windows where CDN loading may be blocked in webview
loader.config({
    paths: {
        vs: '/monaco-editor/min/vs'
    }
})

// Double-click on titlebar to maximize/restore window (macOS behavior)
document.addEventListener('dblclick', (e) => {
    // Check if clicked element or any parent has titlebar-drag class
    let el = e.target
    while (el && el !== document.body) {
        if (el.classList && el.classList.contains('titlebar-drag')) {
            // Don't maximize if clicking on interactive elements
            const tagName = e.target.tagName.toLowerCase()
            if (['button', 'input', 'select', 'textarea', 'a'].includes(tagName)) {
                return
            }
            if (e.target.closest('[role="button"]') || e.target.closest('.no-drag')) {
                return
            }
            WindowToggleMaximise()
            return
        }
        el = el.parentElement
    }
})

const container = document.getElementById('root')

const root = createRoot(container)

root.render(
    <React.StrictMode>
        <ErrorBoundary>
            <App />
        </ErrorBoundary>
    </React.StrictMode>
)
