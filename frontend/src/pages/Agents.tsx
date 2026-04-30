import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Bot } from 'lucide-react';
import { useNodeClient } from '../hooks/useNodeClient';
import { useToastStore } from '../store/toast';
import type { AgentInfo } from '../types/api';

export function Agents() {
  const { t } = useTranslation();
  const client = useNodeClient();
  const addToast = useToastStore((s) => s.addToast);
  const [agents, setAgents] = useState<AgentInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  // 加载代理列表
  useEffect(() => {
    client.listAgents()
      .then(setAgents)
      .catch((e) => {
        const msg = e instanceof Error ? e.message : '加载代理列表失败';
        setError(msg);
        addToast('error', msg);
      })
      .finally(() => setLoading(false));
  }, [client, addToast]);

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <h2 className="text-lg font-semibold text-[var(--text-primary)] mb-6 font-display">{t('agents.title')}</h2>
      {loading ? (
        <div className="text-center py-12 text-[var(--text-secondary)] text-sm animate-pulse">{t('agents.loading')}</div>
      ) : error ? (
        <div className="text-center py-20">
          <Bot className="w-16 h-16 mx-auto mb-4 text-red-300 dark:text-red-700" />
          <div className="text-red-500 text-sm">{error}</div>
        </div>
      ) : agents.length === 0 ? (
        <div className="text-center py-20">
          <Bot className="w-16 h-16 mx-auto mb-4 text-[var(--text-secondary)] opacity-30" />
          <div className="text-[var(--text-secondary)] text-sm">{t('agents.noAgents')}</div>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
          {agents.map((agent) => (
            <div key={agent.id} className="bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl shadow-sm transition-shadow p-5">
              <div className="flex items-start justify-between">
                <div>
                  <div className="flex items-center gap-2.5">
                    <Bot className="w-5 h-5 text-[var(--accent-600)] dark:text-[var(--accent-300)]" />
                    <h3 className="text-sm font-medium text-[var(--text-primary)]">{agent.name}</h3>
                    <span className="text-xs text-[var(--text-secondary)] bg-[var(--bg-secondary)] px-2 py-0.5 rounded-md font-mono">
                      {agent.id}
                    </span>
                  </div>
                  <p className="text-xs text-[var(--text-secondary)] mt-2 ml-[30px]">{agent.description}</p>
                </div>
              </div>
              {agent.skills && agent.skills.length > 0 && (
                <div className="mt-3 ml-[30px] flex flex-wrap gap-1.5">
                  {agent.skills.map((sk) => (
                    <span key={sk} className="px-2 py-0.5 text-xs bg-[var(--accent-50)] dark:bg-[var(--accent-light)] text-[var(--accent-600)] dark:text-[var(--accent-300)] rounded-md">
                      {sk}
                    </span>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
