import type { JournalEvent } from '../../types/journal';

interface Props {
  event: JournalEvent | null;
  onCreateCandidate?: () => void;
  canCreateCandidate?: boolean;
  candidateBusy?: boolean;
}

export function EventDetailPanel({ event, onCreateCandidate, canCreateCandidate = false, candidateBusy = false }: Props) {
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
          {event.quality_event && (
            <div style={{
              marginTop: 12,
              padding: 12,
              border: '1px solid var(--border, rgba(0,0,0,0.08))',
              borderRadius: 10,
              background: 'var(--bg-secondary, #F2F2F7)',
            }}>
              <div style={{ marginBottom: 10, display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
                <span style={{ color: 'var(--text-secondary, #6C6C70)', fontSize: 12 }}>质量事件</span>
                <span style={{ fontFamily: 'JetBrains Mono, monospace', fontSize: 12, color: 'var(--accent-600, #2563EB)' }}>
                  {event.quality_event.name}
                </span>
                {canCreateCandidate && onCreateCandidate && isCandidateWorthy(event) && (
                  <button
                    onClick={onCreateCandidate}
                    disabled={candidateBusy}
                    style={{
                      marginLeft: 'auto',
                      padding: '5px 10px',
                      borderRadius: 8,
                      border: '1px solid var(--accent-border, rgba(37,99,235,0.25))',
                      background: candidateBusy ? 'var(--bg-secondary, #E8E8ED)' : 'var(--accent-subtle, #EFF6FF)',
                      color: 'var(--accent-600, #2563EB)',
                      cursor: candidateBusy ? 'wait' : 'pointer',
                      fontSize: 12,
                      fontWeight: 600,
                    }}
                  >
                    {candidateBusy ? '写入中...' : '加入候选池'}
                  </button>
                )}
              </div>
              <QualityField label="失败类型" value={event.quality_event.failure_type} />
              <QualityField label="重试原因" value={event.quality_event.retry_reason} />
              <QualityField label="最终状态" value={event.quality_event.final_status} />
              <QualityField label="工具决策" value={event.quality_event.tool_decision} />
              <QualityField label="Prompt" value={event.quality_event.prompt} />
              <QualityField label="上下文构建" value={event.quality_event.context_build} />
              <QualityField label="委派" value={event.quality_event.delegation} />
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

function QualityField({ label, value }: { label: string; value: unknown }) {
  if (value == null || value === '') return null;

  const formatted = typeof value === 'string'
    ? value
    : JSON.stringify(value, null, 2);

  return (
    <div style={{ marginBottom: 8 }}>
      <span style={{ color: 'var(--text-secondary, #6C6C70)', fontSize: 12 }}>{label}</span>
      <pre style={{
        margin: '4px 0 0',
        fontFamily: 'JetBrains Mono, monospace',
        fontSize: 12,
        color: 'var(--text-primary, #1C1C1E)',
        whiteSpace: 'pre-wrap',
        wordBreak: 'break-word',
      }}>
        {formatted}
      </pre>
    </div>
  );
}

function isCandidateWorthy(event: JournalEvent): boolean {
  const ev = event.quality_event;
  if (!ev) return false;
  if (ev.final_status === 'fail' || ev.final_status === 'blocked' || ev.final_status === 'needs_user') {
    return true;
  }
  return Boolean(ev.failure_type && ev.failure_type !== 'none' && ev.final_status !== 'pass');
}
