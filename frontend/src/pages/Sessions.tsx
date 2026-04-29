import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Plus, Trash2, Inbox } from 'lucide-react';
import { useSessionStore } from '../store/session';
import { useNodeClient } from '../hooks/useNodeClient';
import { formatDateTime } from '../utils/date';

export function Sessions() {
  const { t } = useTranslation();
  const client = useNodeClient();
  const { sessions, loading, error, fetchSessions, createSession, deleteSession } = useSessionStore();
  const navigate = useNavigate();
  const [newName, setNewName] = useState('');
  const [creating, setCreating] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [deleting, setDeleting] = useState<string | null>(null);

  useEffect(() => {
    fetchSessions(client);
  }, [client, fetchSessions]);

  // 创建新会话
  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newName.trim()) return;
    setCreating(true);
    try {
      const id = await createSession(client, newName.trim());
      setNewName('');
      setShowCreate(false);
      navigate(`/sessions/${id}`);
    } catch {
      // 错误在 store 中处理
    }
    setCreating(false);
  };

  // 删除会话
  const handleDelete = async (id: string) => {
    setDeleting(id);
    await deleteSession(client, id);
    setDeleting(null);
  };

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-lg font-semibold text-[var(--text-primary)] font-display">{t('sessions.title')}</h2>
        <button
          onClick={() => setShowCreate(!showCreate)}
          className="inline-flex items-center gap-1.5 px-4 py-2 text-sm font-medium rounded-xl bg-[var(--accent-600)] text-white hover:bg-[var(--accent-700)] transition-all"
        >
          <Plus className="w-4 h-4" />
          {t('sessions.newSession')}
        </button>
      </div>

      {/* 创建表单 */}
      {showCreate && (
        <form onSubmit={handleCreate} className="mb-6 flex gap-3">
          <input
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            placeholder={t('sessions.sessionName')}
            autoFocus
            className="flex-1 px-4 py-2.5 bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl shadow-sm text-sm text-[var(--text-primary)] placeholder:text-[var(--text-secondary)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent-subtle)] focus:outline-none"
          />
          <button
            type="submit"
            disabled={creating || !newName.trim()}
            className="px-5 py-2.5 text-sm font-medium bg-[var(--accent-600)] text-white rounded-xl hover:bg-[var(--accent-700)] disabled:opacity-50 transition-all"
          >
            {creating ? t('sessions.creating') : t('sessions.create')}
          </button>
          <button
            type="button"
            onClick={() => setShowCreate(false)}
            className="px-4 py-2.5 text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
          >
            {t('sessions.cancel')}
          </button>
        </form>
      )}

      {/* 错误提示 */}
      {error && (
        <div className="mb-4 px-4 py-3 bg-red-50 dark:bg-red-900/10 border border-red-200 dark:border-red-800 rounded-xl text-sm text-red-600 dark:text-red-400">
          {error}
        </div>
      )}

      {/* 会话列表 */}
      {loading ? (
        <div className="text-center py-12 text-[var(--text-secondary)] text-sm animate-pulse">
          {t('common.loading')}
        </div>
      ) : sessions.length === 0 ? (
        <div className="text-center py-20">
          <Inbox className="w-16 h-16 mx-auto mb-4 text-[var(--text-secondary)] opacity-30" />
          <div className="text-[var(--text-secondary)] text-sm">{t('sessions.noSessions')}</div>
          <div className="text-[var(--text-secondary)] opacity-60 text-xs mt-1">{t('sessions.noSessionsHint')}</div>
        </div>
      ) : (
        <div className="space-y-2">
          {sessions.map((s) => (
            <div
              key={s.id}
              className="bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl shadow-sm p-5 flex items-center justify-between transition-all cursor-pointer group"
              onClick={() => navigate(`/sessions/${s.id}`)}
            >
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-3">
                  <span className={`w-2 h-2 rounded-full ${s.is_active ? 'bg-emerald-500' : 'bg-[var(--border-color)]'}`} />
                  <span className="text-sm text-[var(--text-primary)] font-medium truncate">{s.message_count === 0 ? t('sessions.newSession', '新会话') : s.name}</span>
                  <span className="text-xs text-[var(--text-secondary)] bg-[var(--bg-secondary)] px-2 py-0.5 rounded-md">
                    default
                  </span>
                </div>
                <div className="flex items-center gap-4 mt-1.5 ml-5">
                  <span className="text-xs text-[var(--text-secondary)]">
                    {s.message_count} {t('sessions.messages')}
                  </span>
                  <span className="text-xs text-[var(--text-secondary)]">
                    {s.total_tokens} {t('sessions.tokens')}
                  </span>
                  <span className="text-xs text-[var(--text-secondary)]">
                    {formatDateTime(s.last_accessed)}
                  </span>
                </div>
              </div>
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  handleDelete(s.id);
                }}
                disabled={deleting === s.id}
                className="p-2 text-[var(--text-secondary)] hover:text-red-500 dark:hover:text-red-400 opacity-0 group-hover:opacity-100 transition-all rounded-lg hover:bg-red-50 dark:hover:bg-red-900/10"
              >
                {deleting === s.id ? (
                  <span className="text-xs">...</span>
                ) : (
                  <Trash2 className="w-4 h-4" />
                )}
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
