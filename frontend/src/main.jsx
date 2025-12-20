import React from 'react'
import { createRoot } from 'react-dom/client'
import loader from '@monaco-editor/loader'
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

const container = document.getElementById('root')

const root = createRoot(container)

root.render(
    <React.StrictMode>
        <ErrorBoundary>
            <App />
        </ErrorBoundary>
    </React.StrictMode>
)
