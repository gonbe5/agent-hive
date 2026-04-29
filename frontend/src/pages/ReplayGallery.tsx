import { useEffect, useState, useMemo } from 'react';
import { useNodeClient } from '../hooks/useNodeClient';
import type { Session } from '../types/api';
import type { JournalStats } from '../types/journal';
import { ReplayCard } from '../components/replay/ReplayCard';
import { Search, SlidersHorizontal, Hexagon } from 'lucide-react';

type FilterStatus = 'all' | 'success' | 'error' | 'live';
type SortBy = 'newest' | 'longest' | 'most_tools';

const filterOptions: { value: FilterStatus; label: string }[] = [
  { value: 'all', label: '全部' },
  { value: 'success', label: '成功' },
  { value: 'error', label: '失败' },
  { value: 'live', label: '进行中' },
];

const sortOptions: { value: SortBy; label: string }[] = [
  { value: 'newest', label: '最新' },
  { value: 'longest', label: '最长' },
  { value: 'most_tools', label: '最多工具' },
];

export function ReplayGallery() {
  const client = useNodeClient();
  const [sessions, setSessions] = useState<Session[]>([]);
  const [statsMap, setStatsMap] = useState<Record<string, JournalStats | null>>({});
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState('');
  const [filter, setFilter] = useState<FilterStatus>('all');
  const [sortBy, setSortBy] = useState<SortBy>('newest');

  useEffect(() => {
    if (!client) return;
    const load = async () => {
      try {
        const list = await client.listSessions();
        setSessions(list);
        if (list.length > 0) {
          const ids = list.map((s) => s.id);
          const res = await client.getJournalStats(ids);
          setStatsMap(res.stats || {});
        }
      } catch (err) {
        console.error('加载回放列表失败:', err);
      } finally {
        setLoading(false);
      }
    };
    load();
  }, [client]);

  const filtered = useMemo(() => {
    let result = sessions;

    if (search.trim()) {
      const q = search.toLowerCase();
      result = result.filter((s) => s.name?.toLowerCase().includes(q));
    }

    if (filter !== 'all') {
      result = result.filter((s) => {
        const st = statsMap[s.id];
        if (!st) return false;
        if (filter === 'success') return !st.has_error && st.ended_at;
        if (filter === 'error') return st.has_error;
        if (filter === 'live') return !st.ended_at;
        return true;
      });
    }

    result = [...result].sort((a, b) => {
      if (sortBy === 'newest') {
        const ta = a.created_at || a.last_accessed;
        const tb = b.created_at || b.last_accessed;
        return new Date(tb).getTime() - new Date(ta).getTime();
      }
      const sa = statsMap[a.id];
      const sb = statsMap[b.id];
      if (sortBy === 'longest') {
        const da = sa?.started_at && sa?.ended_at ? new Date(sa.ended_at).getTime() - new Date(sa.started_at).getTime() : 0;
        const db = sb?.started_at && sb?.ended_at ? new Date(sb.ended_at).getTime() - new Date(sb.started_at).getTime() : 0;
        return db - da;
      }
      if (sortBy === 'most_tools') {
        return (sb?.tool_call_count || 0) - (sa?.tool_call_count || 0);
      }
      return 0;
    });

    return result;
  }, [sessions, statsMap, search, filter, sortBy]);

  if (loading) {
    return (
      <div style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        height: '60vh',
        color: 'var(--text-secondary)',
      }}>
        <div className="thinking-pulse" style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <Hexagon size={20} color="var(--accent)" />
          <span style={{ fontSize: 14 }}>加载中...</span>
        </div>
      </div>
    );
  }

  return (
    <div style={{ padding: '32px 32px', maxWidth: 1200, margin: '0 auto' }}>
      {/* Header */}
      <div style={{ marginBottom: 28 }}>
        <h1
          className="font-display text-gradient"
          style={{
            fontSize: 28,
            fontWeight: 700,
            margin: '0 0 4px',
            letterSpacing: '-0.03em',
            lineHeight: 1.2,
          }}
        >
          回放剧场
        </h1>
        <p style={{
          fontSize: 13,
          color: 'var(--text-secondary)',
          margin: 0,
        }}>
          观看 Agent 的完整工作过程，实时或回放
        </p>
      </div>

      {/* 工具栏 */}
      <div style={{
        display: 'flex',
        alignItems: 'center',
        gap: 12,
        marginBottom: 24,
        flexWrap: 'wrap',
      }}>
        {/* 搜索框 */}
        <div className="apple-input" style={{
          position: 'relative',
          flex: 1,
          maxWidth: 280,
          display: 'flex',
          alignItems: 'center',
          padding: '0 12px',
        }}>
          <Search size={14} color="var(--text-secondary)" style={{ flexShrink: 0 }} />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="搜索会话..."
            style={{
              width: '100%',
              padding: '8px 8px',
              border: 'none',
              background: 'transparent',
              fontSize: 13,
              outline: 'none',
              color: 'var(--text-primary)',
            }}
          />
        </div>

        {/* 筛选 pill 按钮组 */}
        <div style={{
          display: 'flex',
          gap: 2,
          background: 'var(--bg-secondary)',
          borderRadius: 'var(--radius-btn)',
          padding: 2,
        }}>
          {filterOptions.map((opt) => (
            <button
              key={opt.value}
              onClick={() => setFilter(opt.value)}
              style={{
                padding: '6px 14px',
                fontSize: 12,
                fontWeight: filter === opt.value ? 600 : 400,
                color: filter === opt.value ? '#fff' : 'var(--text-secondary)',
                background: filter === opt.value ? 'var(--accent)' : 'transparent',
                border: 'none',
                borderRadius: 8,
                cursor: 'pointer',
                transition: 'all 150ms ease',
                whiteSpace: 'nowrap',
              }}
            >
              {opt.label}
            </button>
          ))}
        </div>

        {/* 排序 */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <SlidersHorizontal size={14} color="var(--text-secondary)" />
          <div style={{
            display: 'flex',
            gap: 2,
            background: 'var(--bg-secondary)',
            borderRadius: 'var(--radius-btn)',
            padding: 2,
          }}>
            {sortOptions.map((opt) => (
              <button
                key={opt.value}
                onClick={() => setSortBy(opt.value)}
                style={{
                  padding: '6px 12px',
                  fontSize: 12,
                  fontWeight: sortBy === opt.value ? 600 : 400,
                  color: sortBy === opt.value ? '#fff' : 'var(--text-secondary)',
                  background: sortBy === opt.value ? 'var(--accent)' : 'transparent',
                  border: 'none',
                  borderRadius: 8,
                  cursor: 'pointer',
                  transition: 'all 150ms ease',
                  whiteSpace: 'nowrap',
                }}
              >
                {opt.label}
              </button>
            ))}
          </div>
        </div>

        {/* 计数 */}
        <span style={{
          fontSize: 12,
          color: 'var(--text-secondary)',
          marginLeft: 'auto',
          fontVariantNumeric: 'tabular-nums',
        }}>
          {filtered.length} 条记录
        </span>
      </div>

      {/* Grid */}
      {filtered.length === 0 ? (
        <div style={{
          textAlign: 'center',
          padding: '80px 0',
          color: 'var(--text-secondary)',
        }}>
          {/* 六边形装饰空状态 */}
          <div style={{
            display: 'flex',
            justifyContent: 'center',
            gap: 4,
            marginBottom: 16,
            opacity: 0.3,
          }}>
            <Hexagon size={24} color="var(--accent)" />
            <Hexagon size={32} color="var(--accent)" style={{ marginTop: -6 }} />
            <Hexagon size={24} color="var(--accent)" />
          </div>
          <p style={{ fontSize: 15, fontWeight: 500, margin: '0 0 4px' }}>
            {sessions.length === 0 ? '还没有任何会话记录' : '没有匹配的会话'}
          </p>
          <p style={{ fontSize: 13, margin: 0 }}>
            {sessions.length === 0 ? '开始一个新会话，回放将自动出现在这里' : '试试调整筛选条件'}
          </p>
        </div>
      ) : (
        <div style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fill, minmax(260px, 1fr))',
          gap: 20,
        }}>
          {filtered.map((session) => (
            <ReplayCard key={session.id} session={session} stats={statsMap[session.id] ?? null} />
          ))}
        </div>
      )}
    </div>
  );
}
