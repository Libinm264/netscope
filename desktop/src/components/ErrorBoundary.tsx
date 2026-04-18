import { Component, type ReactNode, type ErrorInfo } from "react";

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error("[NetScope] Render error:", error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex h-screen flex-col items-center justify-center gap-4 bg-[#0d0d1a] p-8 text-center">
          <div className="text-4xl">⚠️</div>
          <h1 className="text-xl font-bold text-red-400">NetScope crashed</h1>
          <p className="max-w-md text-sm text-gray-400">
            A rendering error occurred. Open DevTools (right-click → Inspect) to
            see the full stack trace.
          </p>
          <pre className="max-w-xl overflow-auto rounded-lg bg-red-950/30 border border-red-700/30 p-4 text-left text-xs text-red-300">
            {this.state.error.message}
          </pre>
          <button
            className="rounded-lg bg-white/10 px-4 py-2 text-sm text-white hover:bg-white/20"
            onClick={() => this.setState({ error: null })}
          >
            Try again
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}
