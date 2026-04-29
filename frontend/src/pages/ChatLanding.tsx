import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Send, MessageSquare } from 'lucide-react';
import { HiveLogo } from '../layouts/Sidebar';
import { useSessionStore } from '../store/session';
import { useNodeClient } from '../hooks/useNodeClient';

export function ChatLanding() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const client = useNodeClient();
  const sessions = useSessionStore((s) => s.sessions);
  const fetchSessions = useSessionStore((s) => s.fetchSessions);
  const createSession = useSessionStore((s) => s.createSession);
  const [message, setMessage] = useState('');
  const [sending, setSending] = useState(false);

  useEffect(() => {
    fetchSessions(client);
  }, [client, fetchSessions]);

  const handleSend = async () => {
    const text = message.trim();
    if (!text || sending) return;

    setSending(true);
    try {
      // 复用已有空会话，避免重复创建
      const emptySession = sessions.find(s => s.message_count === 0);
      const id = emptySession
        ? emptySession.id
        : await createSession(client, text.slice(0, 30));
      navigate(`/sessions/${id}`, { state: { pendingMessage: text } });
    } catch {
      // 错误已在 store 中处理
    }
    setSending(false);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  const recentSessions = sessions.filter((s) => s.message_count > 0).slice(0, 5);

  return (
    <div className="flex flex-col items-center justify-center h-full px-4">
      <div className="w-full max-w-xl text-center">
        {/* Logo + 欢迎文案 */}
        <HiveLogo className="w-14 h-14 mx-auto mb-4" />
        <h1 className="text-xl font-semibold text-[var(--text-primary)] mb-1 font-display">
          {t('chatLanding.title')}
        </h1>
        <p className="text-sm text-[var(--text-secondary)] mb-8">
          {t('chatLanding.subtitle')}
        </p>

        {/* 输入框 */}
        <div className="relative">
          <textarea
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder={t('chat.inputPlaceholder')}
            rows={3}
            className="w-full px-4 py-3 pr-12 text-sm rounded-xl border border-[var(--border-color)] bg-[var(--bg-card)] text-[var(--text-primary)] placeholder:text-[var(--text-secondary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)] focus:border-[var(--accent)] resize-none "
          />
          <button
            onClick={handleSend}
            disabled={!message.trim() || sending}
            className="absolute right-3 bottom-3 p-1.5 rounded-lg text-[var(--text-secondary)] hover:text-[var(--accent-600)] disabled:opacity-30 transition-colors"
          >
            <Send className="w-4 h-4" />
          </button>
        </div>

        {/* 最近会话 */}
        {recentSessions.length > 0 && (
          <div className="mt-8">
            <div className="flex items-center gap-1.5 justify-center mb-3">
              <MessageSquare className="w-3 h-3 text-[var(--text-secondary)]" />
              <span className="text-xs text-[var(--text-secondary)]">
                {t('chatLanding.recentSessions')}
              </span>
            </div>
            <div className="flex flex-wrap gap-2 justify-center">
              {recentSessions.map((s) => (
                <button
                  key={s.id}
                  onClick={() => navigate(`/sessions/${s.id}`)}
                  className="px-3 py-1.5 text-xs rounded-lg border border-[var(--border-color)] bg-[var(--bg-card)] text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:border-[var(--accent-300)] dark:hover:border-[var(--accent-700)] transition-colors truncate max-w-[200px]"
                >
                  {s.name || s.id.slice(0, 8)}
                </button>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
