import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TrendingUp, Cpu, DollarSign } from 'lucide-react';
import { useNodeClient } from '../../hooks/useNodeClient';
import { useToastStore } from '../../store/toast';
import type { UsageSummary } from '../../types/api';

export function UsageStats() {
  const { t } = useTranslation();
  const client = useNodeClient();
  const addToast = useToastStore((s) => s.addToast);
  const [summary, setSummary] = useState<UsageSummary | null>(null);
  const [byModel, setByModel] = useState<Record<string, { tokens: number; cost_usd: number }>>({});
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      try {
        const [s, m] = await Promise.all([
          client.adminGetUsageSummary().catch(() => null),
          client.adminGetUsageByModel().catch(() => ({ by_model: {} })),
        ]);
        setSummary(s);
        setByModel(m?.by_model ?? {});
      } catch (e: unknown) {
        addToast('error', e instanceof Error ? e.message : '加载用量统计失败');
      } finally {
        setLoading(false);
      }
    };
    load();
  }, [client]);

  const modelEntries = Object.entries(byModel).sort((a, b) => b[1].tokens - a[1].tokens);

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <div className="mb-6">
        <h1 className="text-xl font-semibold text-[var(--text-primary)]">{t('admin.usage', '用量统计')}</h1>
        <p className="text-sm text-[var(--text-secondary)] mt-1">{t('admin.usageDesc', '查看 Token 消耗和成本概览')}</p>
      </div>

      {loading ? (
        <div className="text-center py-12 text-[var(--text-secondary)] text-sm animate-pulse">{t('common.loading', '加载中...')}</div>
      ) : (
        <>
          {/* 汇总卡片 */}
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-8">
            <div className="rounded-xl border border-[var(--border-color)] bg-[var(--bg-card)] p-5">
              <div className="flex items-center gap-3 mb-2">
                <div className="p-2 rounded-lg bg-[var(--accent-100)] dark:bg-[var(--accent-light)]">
                  <Cpu className="w-4 h-4 text-[var(--accent-600)] dark:text-[var(--accent-300)]" />
                </div>
                <span className="text-sm text-[var(--text-secondary)]">{t('admin.totalTokens', '总 Token 数')}</span>
              </div>
              <div className="text-2xl font-bold text-[var(--text-primary)]">
                {summary ? (summary.total_tokens ?? 0).toLocaleString() : '—'}
              </div>
            </div>

            <div className="rounded-xl border border-[var(--border-color)] bg-[var(--bg-card)] p-5">
              <div className="flex items-center gap-3 mb-2">
                <div className="p-2 rounded-lg bg-[var(--accent-100)] dark:bg-[var(--accent-light)]">
                  <DollarSign className="w-4 h-4 text-[var(--accent-600)] dark:text-[var(--accent-300)]" />
                </div>
                <span className="text-sm text-[var(--text-secondary)]">{t('admin.totalCost', '总成本 (USD)')}</span>
              </div>
              <div className="text-2xl font-bold text-[var(--text-primary)]">
                {summary ? `$${(summary.total_cost_usd ?? 0).toFixed(4)}` : '—'}
              </div>
            </div>

            <div className="rounded-xl border border-[var(--border-color)] bg-[var(--bg-card)] p-5">
              <div className="flex items-center gap-3 mb-2">
                <div className="p-2 rounded-lg bg-[var(--accent-100)] dark:bg-[var(--accent-light)]">
                  <TrendingUp className="w-4 h-4 text-[var(--accent-600)] dark:text-[var(--accent-300)]" />
                </div>
                <span className="text-sm text-[var(--text-secondary)]">{t('admin.models', '使用模型数')}</span>
              </div>
              <div className="text-2xl font-bold text-[var(--text-primary)]">
                {modelEntries.length}
              </div>
            </div>
          </div>

          {/* 按模型明细 */}
          {modelEntries.length > 0 && (
            <div className="rounded-xl border border-[var(--border-color)] overflow-hidden">
              <div className="px-4 py-3 bg-[var(--bg-secondary)] border-b border-[var(--border-color)]">
                <h2 className="text-sm font-medium text-[var(--text-primary)]">{t('admin.byModel', '按模型统计')}</h2>
              </div>
              <table className="w-full text-sm">
                <thead className="bg-[var(--bg-secondary)]">
                  <tr>
                    <th className="px-4 py-3 text-left font-medium text-[var(--text-secondary)]">{t('admin.model', '模型')}</th>
                    <th className="px-4 py-3 text-right font-medium text-[var(--text-secondary)]">{t('admin.tokens', 'Tokens')}</th>
                    <th className="px-4 py-3 text-right font-medium text-[var(--text-secondary)]">{t('admin.cost', '成本 (USD)')}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--border-color)]">
                  {modelEntries.map(([model, stats]) => (
                    <tr key={model} className="hover:bg-[var(--bg-secondary)] transition-colors">
                      <td className="px-4 py-3 font-mono text-xs text-[var(--text-primary)]">{model}</td>
                      <td className="px-4 py-3 text-right text-[var(--text-primary)]">{stats.tokens.toLocaleString()}</td>
                      <td className="px-4 py-3 text-right text-[var(--text-primary)]">${stats.cost_usd.toFixed(4)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {modelEntries.length === 0 && (
            <div className="text-center py-12 text-[var(--text-secondary)] text-sm">
              {t('admin.noUsageData', '暂无用量数据（成本追踪可能未启用）')}
            </div>
          )}
        </>
      )}
    </div>
  );
}
