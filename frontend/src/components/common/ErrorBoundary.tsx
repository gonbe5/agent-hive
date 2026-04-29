import { Component } from 'react';
import type { ReactNode, ErrorInfo } from 'react';
import i18n from '../../i18n';

interface Props {
  children: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

/** 全局错误边界，防止子组件异常导致白屏 */
export class ErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('[ErrorBoundary] 捕获到渲染错误:', error, info.componentStack);
  }

  handleRetry = () => {
    this.setState({ hasError: false, error: null });
  };

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex items-center justify-center h-screen bg-[var(--bg-primary)]">
          <div className="text-center max-w-md px-6">
            <div className="text-5xl mb-4">:(</div>
            <h2 className="text-lg font-semibold text-[var(--text-primary)] mb-2">
              {i18n.t('errorBoundary.title', { defaultValue: 'Something went wrong' })}
            </h2>
            <p className="text-sm text-[var(--text-secondary)] mb-6">
              {this.state.error?.message || i18n.t('errorBoundary.fallback', { defaultValue: 'An unexpected error occurred' })}
            </p>
            <button
              onClick={this.handleRetry}
              className="px-5 py-2.5 text-sm font-medium bg-[var(--danger)] text-white rounded-xl hover:opacity-90 transition-all"
            >
              {i18n.t('errorBoundary.retry', { defaultValue: 'Retry' })}
            </button>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
