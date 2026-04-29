import { Component, Suspense } from 'react';
import type { ReactNode, ErrorInfo } from 'react';

interface Props {
  children: ReactNode;
  messageTimestamp?: string;
  messageRole?: string;
  messageContentPreview?: string;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

/**
 * 单条消息级渲染保护。
 *
 * 为什么需要：
 *   - Streamdown 内部用 React.Suspense（见 streamdown/dist/chunk-BO2N2NFS.js）。
 *     若外层无 Suspense boundary，某些历史消息内容在渲染时 suspend 会导致
 *     整个 MessageList 子树静默空白（P0：发消息后历史消失）。
 *   - Tool 适配层换成 Radix Collapsible 后，某些历史 tool_call 的结构异常
 *     可能触发渲染时 throw，全局 ErrorBoundary 是全屏 fallback 不合适。
 *
 * 策略：
 *   - 每条消息独立挂一个 boundary：挂起 → Suspense fallback（保持占位）；
 *     报错 → 降级为失败提示卡片，其它消息继续正常渲染。
 *   - console.error 打印 messageTimestamp + role + 内容预览 + 错误栈，
 *     方便定位到底哪一条消息触发。
 */
class MessageErrorBoundary extends Component<Props, State> {
  constructor(props: Props) {
    super(props);
    this.state = { hasError: false, error: null };
  }

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error(
      '[MessageBubbleBoundary] 单条消息渲染失败',
      {
        timestamp: this.props.messageTimestamp,
        role: this.props.messageRole,
        preview: this.props.messageContentPreview,
      },
      error,
      info.componentStack,
    );
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="px-4 py-2">
          <div className="max-w-[85%] rounded-lg border border-[var(--danger)]/30 bg-[var(--danger)]/5 px-3 py-2 text-xs text-[var(--text-secondary)]">
            <div className="font-medium text-[var(--danger)] mb-1">此消息渲染失败</div>
            <div className="font-mono truncate" title={this.state.error?.message}>
              {this.state.error?.message ?? 'unknown error'}
            </div>
          </div>
        </div>
      );
    }
    return this.props.children;
  }
}

const SuspenseFallback = () => (
  <div className="px-4 py-1">
    <div className="max-w-[85%] h-6 rounded bg-[var(--bg-secondary)]/40 animate-pulse" />
  </div>
);

export function MessageBubbleBoundary({
  children,
  messageTimestamp,
  messageRole,
  messageContentPreview,
}: Props) {
  return (
    <MessageErrorBoundary
      messageTimestamp={messageTimestamp}
      messageRole={messageRole}
      messageContentPreview={messageContentPreview}
    >
      <Suspense fallback={<SuspenseFallback />}>{children}</Suspense>
    </MessageErrorBoundary>
  );
}

export default MessageBubbleBoundary;
