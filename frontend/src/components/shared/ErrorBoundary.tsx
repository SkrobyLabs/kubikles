import React from 'react';

class ErrorBoundary extends React.Component {
    constructor(props) {
        super(props);
        this.state = { hasError: false, error: null, errorInfo: null, dismissed: false, copied: false };
    }

    static getDerivedStateFromError(error) {
        return { hasError: true, dismissed: false };
    }

    componentDidCatch(error, errorInfo) {
        this.setState({ error, errorInfo });
        console.error("Uncaught error:", error, errorInfo);
    }

    handleDismiss = () => {
        this.setState({ hasError: false, error: null, errorInfo: null, dismissed: true });
    };

    handleCopy = () => {
        const text = [
            this.state.error?.toString(),
            this.state.errorInfo?.componentStack
        ].filter(Boolean).join('\n');
        navigator.clipboard.writeText(text);
        this.setState({ copied: true });
        setTimeout(() => this.setState({ copied: false }), 2000);
    };

    render() {
        if (this.state.hasError) {
            const errorText = this.state.error?.toString() || 'Unknown error';
            const stack = this.state.errorInfo?.componentStack;

            return (
                <div className="h-screen w-full flex items-center justify-center bg-surface p-8">
                    <div className="max-w-2xl w-full bg-surface-secondary border border-border rounded-lg shadow-lg overflow-hidden">
                        <div className="px-5 py-4 border-b border-border flex items-center justify-between">
                            <div className="flex items-center gap-2">
                                <span className="text-red-400 text-lg">●</span>
                                <h2 className="text-base font-semibold text-primary">Something went wrong</h2>
                            </div>
                            <div className="flex items-center gap-2">
                                <button
                                    onClick={this.handleCopy}
                                    className="px-3 py-1.5 text-xs font-medium text-secondary bg-surface hover:bg-hover border border-border rounded transition-colors"
                                >
                                    {this.state.copied ? 'Copied' : 'Copy'}
                                </button>
                                <button
                                    onClick={this.handleDismiss}
                                    className="px-3 py-1.5 text-xs font-medium text-white bg-accent hover:bg-accent/80 rounded transition-colors"
                                >
                                    Dismiss
                                </button>
                            </div>
                        </div>
                        <div className="p-5">
                            <p className="text-sm font-medium text-red-400 mb-3 select-text">{errorText}</p>
                            {stack && (
                                <pre className="text-xs text-tertiary font-mono whitespace-pre-wrap select-text bg-surface rounded border border-border p-3 max-h-80 overflow-auto">
                                    {stack.trim()}
                                </pre>
                            )}
                        </div>
                    </div>
                </div>
            );
        }

        return this.props.children;
    }
}

export default ErrorBoundary;
