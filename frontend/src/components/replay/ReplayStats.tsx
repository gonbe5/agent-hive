import { useEffect, useState } from 'react';
import type { JournalEvent } from '../../types/journal';

interface Props {
  events: JournalEvent[];
  startedAt?: string;
  endedAt?: string;
}

export function ReplayStats({ events, startedAt, endedAt }: Props) {
  const [liveNow, setLiveNow] = useState(0);
  const isLive = !!startedAt && !endedAt;

  useEffect(() => {
    if (!isLive) return;
    const timer = window.setInterval(() => setLiveNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [isLive]);

  const toolCalls = events.filter((e) => e.type === 'tool_call').length;
  const fileChanges = events.filter((e) => e.type === 'file_change').length;
  const decisions = events.filter((e) => e.type === 'decision').length;
  const hasError = events.some((e) => e.is_error);

  let duration = '';
  if (startedAt) {
    const start = new Date(startedAt).getTime();
    const end = endedAt ? new Date(endedAt).getTime() : liveNow;
    const secs = Math.max(0, Math.floor((end - start) / 1000));
    const mins = Math.floor(secs / 60);
    const remainSecs = secs % 60;
    duration = mins > 0 ? `${mins}m${remainSecs}s` : `${remainSecs}s`;
  }

  const items = [
    { icon: '⏱', label: duration || '--', title: '总耗时' },
    { icon: '🔧', label: String(toolCalls), title: '工具调用' },
    { icon: '📄', label: String(fileChanges), title: '文件变更' },
    { icon: '💭', label: String(decisions), title: '决策' },
    { icon: hasError ? '❌' : '✅', label: hasError ? '失败' : '成功', title: '状态' },
  ];

  return (
    <div style={{
      display: 'flex',
      gap: 16,
      padding: '8px 16px',
      fontFamily: 'DM Sans, sans-serif',
      fontSize: 13,
      color: 'var(--text-secondary, #6C6C70)',
      borderBottom: '1px solid var(--border, rgba(0,0,0,0.08))',
    }}>
      {items.map((item) => (
        <span key={item.title} title={item.title} style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
          <span style={{ fontSize: 14 }}>{item.icon}</span>
          <span>{item.label}</span>
        </span>
      ))}
    </div>
  );
}
