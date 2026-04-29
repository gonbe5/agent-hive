import { useEffect, useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useNodeClient } from '../../hooks/useNodeClient';
import { useToastStore } from '../../store/toast';
import type { RuntimeConfig } from '../../types/api';

/** 纳秒转可读字符串 */
function formatNanosToStr(nanos: number): string {
  const seconds = nanos / 1e9;
  if (seconds >= 60) {
    return `${Math.round(seconds / 60)}m`;
  }
  return `${Math.round(seconds)}s`;
}

export function AgentTimeoutSettings() {
  const { t } = useTranslation();
  const client = useNodeClient();
  const addToast = useToastStore((s) => s.addToast);

  const [taskTimeout, setTaskTimeout] = useState('30m');
  const [shellTimeout, setShellTimeout] = useState('10s');
  const [loading, setLoading] = useState(true);
  const [applying, setApplying] = useState(false);

  const loadConfig = useCallback(async () => {
    setLoading(true);
    try {
      const cfg: RuntimeConfig = await client.getRuntimeConfig();
      if (cfg.agent?.timeout) {
        setTaskTimeout(formatNanosToStr(cfg.agent.timeout));
      }
      if (cfg.agent?.shell_timeout) {
        setShellTimeout(formatNanosToStr(cfg.agent.shell_timeout));
      }
    } catch (e) {
      const msg = e instanceof Error ? e.message : t('runtimeConfig.loadFailed');
      addToast('error', msg);
    } finally {
      setLoading(false);
    }
  }, [client, addToast, t]);

  useEffect(() => {
    loadConfig();
  }, [loadConfig]);

  const handleApply = async () => {
    setApplying(true);
    try {
      await client.updateRuntimeConfig({
        agent: {
          timeout: taskTimeout,
          shell_timeout: shellTimeout,
        },
      });
      addToast('success', t('runtimeConfig.applySuccess'));
    } catch (e) {
      const msg = e instanceof Error ? e.message : t('runtimeConfig.applyFailed');
      addToast('error', msg);
    } finally {
      setApplying(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12 text-[var(--text-secondary)]">
        {t('common.loading')}
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <SettingsSection title={t('runtimeConfig.agentSettings')}>
        <div className="divide-y divide-[var(--border-color)]">
          <div className="flex items-center justify-between px-5 py-4">
            <span className="text-sm text-[var(--text-secondary)]">{t('runtimeConfig.taskTimeout')}</span>
            <select
              value={taskTimeout}
              onChange={(e) => setTaskTimeout(e.target.value)}
              className="px-2 py-1 text-sm rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)] focus:border-[var(--accent)]"
            >
              <option value="10m">10m</option>
              <option value="30m">30m</option>
              <option value="60m">60m</option>
            </select>
          </div>

          <div className="flex items-center justify-between px-5 py-4">
            <span className="text-sm text-[var(--text-secondary)]">{t('runtimeConfig.shellTimeout')}</span>
            <select
              value={shellTimeout}
              onChange={(e) => setShellTimeout(e.target.value)}
              className="px-2 py-1 text-sm rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)] focus:border-[var(--accent)]"
            >
              <option value="10s">10s</option>
              <option value="30s">30s</option>
              <option value="60s">60s</option>
            </select>
          </div>
        </div>
      </SettingsSection>

      <div className="flex gap-3">
        <button
          onClick={handleApply}
          disabled={applying}
          className="flex-1 px-4 py-2.5 text-sm font-medium text-white bg-[var(--accent-600)] hover:bg-[var(--accent-700)] disabled:opacity-50 rounded-xl transition-colors"
        >
          {applying ? t('common.loading') : t('runtimeConfig.apply')}
        </button>
      </div>
    </div>
  );
}

function SettingsSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl overflow-hidden shadow-sm">
      <div className="px-5 py-4 border-b border-[var(--border-color)] flex items-center justify-between">
        <span className="text-sm font-medium text-[var(--text-primary)]">{title}</span>
      </div>
      {children}
    </div>
  );
}
