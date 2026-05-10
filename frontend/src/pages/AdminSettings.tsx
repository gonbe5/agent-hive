import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNodeClient } from '../hooks/useNodeClient';
import { useAppStore } from '../store/app';
import { useToastStore } from '../store/toast';
import { IMChannelSettings } from '../components/settings/IMChannelSettings';
import { RemoteAgentsSettings } from '../components/settings/RemoteAgentsSettings';
import { ExternalResourcesSettings } from '../components/settings/ExternalResourcesSettings';
import { PermissionRulesSettings } from '../components/settings/PermissionRulesSettings';
import { AgentTimeoutSettings } from '../components/settings/AgentTimeoutSettings';
import { MCPServersSettings } from '../components/settings/MCPServersSettings';
import { ExecRulesSettings } from '../components/settings/ExecRulesSettings';
import type { Health } from '../types/api';

type TabType = 'system' | 'security' | 'integrations';

export function AdminSettings() {
  const { t } = useTranslation();
  const client = useNodeClient();
  const mode = useAppStore((s) => s.mode);
  const addToast = useToastStore((s) => s.addToast);
  const [health, setHealth] = useState<Health | null>(null);
  const [activeTab, setActiveTab] = useState<TabType>('system');

  useEffect(() => {
    client.health().then(setHealth).catch((e) => {
      const msg = e instanceof Error ? e.message : t('settings.healthFetchFailed', '获取节点状态失败');
      addToast('error', msg);
    });
  }, [client, addToast, t]);

  const tabs: { key: TabType; label: string }[] = [
    { key: 'system', label: t('adminSettings.system') },
    { key: 'security', label: t('adminSettings.security') },
    { key: 'integrations', label: t('adminSettings.integrations') },
  ];

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <h2 className="text-lg font-semibold text-[var(--text-primary)] mb-6 font-display">
        {t('nav.adminSettings')}
      </h2>

      {/* Tab 导航 */}
      <div className="flex gap-4 border-b border-[var(--border-color)] mb-6">
        {tabs.map((tab) => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`px-4 py-2 text-sm font-medium transition-colors border-b-2 ${
              activeTab === tab.key
                ? 'border-[var(--accent-600)] text-[var(--accent-600)]'
                : 'border-transparent text-[var(--text-secondary)] hover:text-[var(--text-primary)]'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Tab 内容 */}
      {activeTab === 'system' && (
        <div className="space-y-6">
          {/* 节点信息（只读） */}
          <SettingsSection title={t('settings.nodeInfo')}>
            <SettingsRow label={t('settings.nodeId')} value={client.nodeId} />
            <SettingsRow label={t('settings.mode')} value={mode} />
            <SettingsRow label={t('settings.status')} value={health?.status || 'unknown'} />
            {health?.version && <SettingsRow label={t('settings.version')} value={health.version} />}
            {health?.uptime != null && (
              <SettingsRow label={t('settings.uptime')} value={`${Math.floor(health.uptime / 60)}m`} />
            )}
          </SettingsSection>

          {/* Agent 超时设置 */}
          <AgentTimeoutSettings />
        </div>
      )}

      {activeTab === 'security' && (
        <div className="space-y-6">
          <PermissionRulesSettings />
          <ExecRulesSettings />
        </div>
      )}

      {activeTab === 'integrations' && (
        <div className="space-y-8">
          <IMChannelSettings />
          <MCPServersSettings />
          <ExternalResourcesSettings />
          <RemoteAgentsSettings />
        </div>
      )}
    </div>
  );
}

function SettingsSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl shadow-sm overflow-hidden">
      <div className="px-5 py-4 border-b border-[var(--border-color)]">
        <span className="text-sm font-medium text-[var(--text-primary)]">{title}</span>
      </div>
      <div className="divide-y divide-[var(--border-color)]">{children}</div>
    </div>
  );
}

function SettingsRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between px-5 py-4">
      <span className="text-sm text-[var(--text-secondary)]">{label}</span>
      <span className="text-sm font-mono text-[var(--text-primary)]">{value}</span>
    </div>
  );
}
