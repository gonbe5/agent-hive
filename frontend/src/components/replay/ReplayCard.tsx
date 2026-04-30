import { useNavigate } from 'react-router-dom';
import { useEffect, useState } from 'react';
import { Clock, Wrench, FileText, MessageSquare, CheckCircle2, XCircle, Loader2, Radio, GitBranch, ShieldAlert } from 'lucide-react';
import type { Session } from '../../types/api';
import type { JournalStats } from '../../types/journal';
import { AgentCharacter } from './AgentCharacter';

interface Props {
  session: Session;
  stats: JournalStats | null;
}

export function ReplayCard({ session, stats }: Props) {
  const navigate = useNavigate();
  const isLive = stats != null && !stats.ended_at;
  const [liveNow, setLiveNow] = useState(0);

  useEffect(() => {
    if (!isLive) return;
    const timer = window.setInterval(() => setLiveNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [isLive]);

  let duration = '';
  if (stats?.started_at) {
    const start = new Date(stats.started_at).getTime();
    const end = stats.ended_at ? new Date(stats.ended_at).getTime() : liveNow;
    const totalSecs = Math.max(0, Math.floor((end - start) / 1000));
    const days = Math.floor(totalSecs / 86400);
    const hours = Math.floor((totalSecs % 86400) / 3600);
    const mins = Math.floor((totalSecs % 3600) / 60);
    const secs = totalSecs % 60;
    // 超过 24h 按天显示；超过 1h 按小时；其他按分钟/秒。只保留 2 段，避免过长
    if (days > 0) {
      duration = `${days}d${hours}h`;
    } else if (hours > 0) {
      duration = `${hours}h${mins}m`;
    } else if (mins > 0) {
      duration = `${mins}m${secs}s`;
    } else {
      duration = `${secs}s`;
    }
  }

  const hasData = stats != null;
  const isError = hasData && stats.has_error;
  const isSuccess = hasData && !!stats.ended_at && !stats.has_error;

  // 状态对应的颜色和图标
  const statusConfig = !hasData
    ? { icon: Loader2, color: 'var(--text-secondary)', label: '加载中', className: 'thinking-pulse' }
    : isError
      ? { icon: XCircle, color: '#DC2626', label: '出错', className: '' }
      : isSuccess
        ? { icon: CheckCircle2, color: '#059669', label: '完成', className: '' }
        : { icon: Radio, color: 'var(--accent)', label: '进行中', className: 'thinking-pulse' };

  const StatusIcon = statusConfig.icon;

  return (
    <button
      className="apple-card"
      onClick={() => navigate(`/sessions/${session.id}/replay`)}
      style={{
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        cursor: 'pointer',
        textAlign: 'left',
        width: '100%',
        transition: 'box-shadow 200ms ease, transform 200ms ease',
        padding: 0,
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.boxShadow = '0 8px 24px rgba(0,0,0,0.1)';
        e.currentTarget.style.transform = 'translateY(-3px)';
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.boxShadow = 'var(--card-shadow)';
        e.currentTarget.style.transform = 'none';
      }}
    >
      {/* 缩略图区域：带背景色区分 */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: '20px 0 12px',
        position: 'relative',
        background: 'var(--accent-subtle)',
        borderBottom: '1px solid var(--border-color)',
      }}>
        <AgentCharacter state={isLive ? 'running' : isError ? 'error' : isSuccess ? 'success' : 'idle'} size={64} />

        {/* 状态图标 */}
        <span
          className={statusConfig.className}
          style={{
            position: 'absolute',
            top: 10,
            right: 10,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <StatusIcon size={16} color={statusConfig.color} aria-label={statusConfig.label} />
        </span>

        {/* 时长标签 */}
        {duration && (
          <span style={{
            position: 'absolute',
            top: 10,
            left: 10,
            fontSize: 11,
            fontWeight: 500,
            color: 'var(--text-secondary)',
            background: 'var(--bg-card)',
            padding: '2px 8px',
            borderRadius: 'var(--radius-badge)',
            display: 'flex',
            alignItems: 'center',
            gap: 3,
            border: '1px solid var(--border-color)',
          }}>
            <Clock size={10} />
            {duration}
          </span>
        )}
      </div>

      {/* 信息区域 */}
      <div style={{ padding: '12px 14px 14px' }}>
        <div style={{
          fontSize: 14,
          fontWeight: 600,
          color: 'var(--text-primary)',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          marginBottom: 8,
          letterSpacing: '-0.01em',
        }}>
          {session.name || '未命名会话'}
        </div>

        {hasData && (
          <div style={{
            display: 'flex',
            gap: 12,
            fontSize: 12,
            color: 'var(--text-secondary)',
            marginBottom: 8,
          }}>
            <span style={{ display: 'flex', alignItems: 'center', gap: 3 }}>
              <Wrench size={12} color="var(--accent)" />
              {stats.tool_call_count}
            </span>
            <span style={{ display: 'flex', alignItems: 'center', gap: 3 }}>
              <FileText size={12} />
              {stats.file_change_count}
            </span>
            <span style={{ display: 'flex', alignItems: 'center', gap: 3 }}>
              <MessageSquare size={12} />
              {stats.decision_count}
            </span>
          </div>
        )}

        {hasData && ((stats.quality_error_count ?? 0) > 0 || (stats.dangerous_count ?? 0) > 0 || (stats.delegation_count ?? 0) > 0) && (
          <div style={{
            display: 'flex',
            gap: 6,
            flexWrap: 'wrap',
            marginBottom: 8,
          }}>
            {(stats.quality_error_count ?? 0) > 0 && (
              <ReplayBadge tone="error" label={`质量 ${stats.quality_error_count}`} />
            )}
            {(stats.dangerous_count ?? 0) > 0 && (
              <ReplayBadge tone="danger" label={`危险 ${stats.dangerous_count}`} icon="danger" />
            )}
            {(stats.delegation_count ?? 0) > 0 && (
              <ReplayBadge tone="info" label={`委派 ${stats.delegation_count}`} icon="branch" />
            )}
          </div>
        )}

        <div style={{
          fontSize: 11,
          color: 'var(--text-secondary)',
          fontVariantNumeric: 'tabular-nums',
        }}>
          {session.created_at
            ? new Date(session.created_at).toLocaleDateString()
            : new Date(session.last_accessed).toLocaleDateString()}
        </div>
      </div>

      {/* 底部品牌色条：进行中的会话 */}
      {isLive && (
        <div style={{
          height: 2,
          background: 'linear-gradient(90deg, transparent, var(--accent), transparent)',
          backgroundSize: '200% 100%',
          animation: 'tool-progress 1.4s ease-in-out infinite',
        }} />
      )}
    </button>
  );
}

function ReplayBadge({ tone, label, icon }: { tone: 'error' | 'danger' | 'info'; label: string; icon?: 'danger' | 'branch' }) {
  const color = tone === 'error' ? '#DC2626' : tone === 'danger' ? '#D97706' : 'var(--accent)';
  const Icon = icon === 'danger' ? ShieldAlert : icon === 'branch' ? GitBranch : null;

  return (
    <span style={{
      display: 'inline-flex',
      alignItems: 'center',
      gap: 3,
      fontSize: 10,
      fontWeight: 600,
      color,
      background: 'var(--bg-secondary)',
      border: `1px solid ${color}33`,
      borderRadius: 'var(--radius-badge)',
      padding: '2px 6px',
    }}>
      {Icon && <Icon size={10} />}
      {label}
    </span>
  );
}
