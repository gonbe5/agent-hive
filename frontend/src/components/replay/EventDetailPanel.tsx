import type { JournalEvent } from '../../types/journal';

interface Props {
  event: JournalEvent | null;
}

export function EventDetailPanel({ event }: Props) {
  if (!event) {
    return (
      <div style={{ padding: 16, color: 'var(--text-secondary, #6C6C70)', fontSize: 13, fontFamily: 'DM Sans, sans-serif' }}>
        选择一个事件查看详情
      </div>
    );
  }

  return (
    <div style={{ padding: 16, fontFamily: 'DM Sans, sans-serif', fontSize: 13, overflowY: 'auto' }}>
      {event.type === 'tool_call' && (
        <>
          <div style={{ marginBottom: 8 }}>
            <span style={{ color: 'var(--text-secondary, #6C6C70)', fontSize: 12 }}>工具</span>
            <div style={{ fontFamily: 'JetBrains Mono, monospace', fontSize: 13, color: 'var(--accent-600, #2563EB)' }}>
              {event.tool_name}
            </div>
          </div>
          {event.arguments && (
            <div style={{ marginBottom: 8 }}>
              <span style={{ color: 'var(--text-secondary, #6C6C70)', fontSize: 12 }}>参数</span>
              <pre style={{
                fontFamily: 'JetBrains Mono, monospace',
                fontSize: 12,
                background: 'var(--bg-secondary, #E8E8ED)',
                padding: 10,
                borderRadius: 8,
                overflowX: 'auto',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
                maxHeight: 200,
              }}>
                {formatJSON(event.arguments)}
              </pre>
            </div>
          )}
          {event.result && (
            <div style={{ marginBottom: 8 }}>
              <span style={{ color: 'var(--text-secondary, #6C6C70)', fontSize: 12 }}>结果</span>
              <pre style={{
                fontFamily: 'JetBrains Mono, monospace',
                fontSize: 12,
                background: 'var(--bg-secondary, #E8E8ED)',
                padding: 10,
                borderRadius: 8,
                overflowX: 'auto',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-all',
                maxHeight: 200,
              }}>
                {event.result.length > 500 ? event.result.slice(0, 500) + '...' : event.result}
              </pre>
            </div>
          )}
          <div style={{ display: 'flex', gap: 16, fontSize: 12, color: 'var(--text-secondary, #6C6C70)' }}>
            {event.duration_ms != null && <span>耗时: {event.duration_ms}ms</span>}
            <span>状态: {event.is_error ? '❌ 失败' : '✅ 成功'}</span>
          </div>
        </>
      )}

      {event.type === 'file_change' && (
        <>
          <div style={{ marginBottom: 8 }}>
            <span style={{ color: 'var(--text-secondary, #6C6C70)', fontSize: 12 }}>操作</span>
            <div style={{ fontFamily: 'JetBrains Mono, monospace', fontSize: 13 }}>
              {event.action === 'create' ? '📄 创建' : event.action === 'delete' ? '🗑️ 删除' : '✏️ 编辑'}
            </div>
          </div>
          <div style={{ marginBottom: 8 }}>
            <span style={{ color: 'var(--text-secondary, #6C6C70)', fontSize: 12 }}>文件</span>
            <div style={{ fontFamily: 'JetBrains Mono, monospace', fontSize: 13, color: 'var(--accent-600, #2563EB)' }}>
              {event.file_path}
            </div>
          </div>
          {event.summary && (
            <div>
              <span style={{ color: 'var(--text-secondary, #6C6C70)', fontSize: 12 }}>摘要</span>
              <div>{event.summary}</div>
            </div>
          )}
        </>
      )}

      {event.type === 'decision' && (
        <>
          <div style={{ marginBottom: 8 }}>
            <span style={{ color: 'var(--text-secondary, #6C6C70)', fontSize: 12 }}>决策</span>
            <div style={{ fontSize: 14 }}>{event.decision}</div>
          </div>
          {event.reason && (
            <div>
              <span style={{ color: 'var(--text-secondary, #6C6C70)', fontSize: 12 }}>原因</span>
              <div>{event.reason}</div>
            </div>
          )}
        </>
      )}

      <div style={{ marginTop: 12, fontSize: 11, color: 'var(--text-secondary, #6C6C70)' }}>
        {new Date(event.timestamp).toLocaleTimeString()}
      </div>
    </div>
  );
}

function formatJSON(s: string): string {
  try {
    return JSON.stringify(JSON.parse(s), null, 2);
  } catch {
    return s;
  }
}
