import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNodeClient } from '../../hooks/useNodeClient';
import { useToastStore } from '../../store/toast';
import type { RemoteAgentConfig, RemoteAgentHealth } from '../../types/api';

export function RemoteAgentsSettings() {
  const { t } = useTranslation();
  const client = useNodeClient();
  const showToast = useToastStore((s) => s.addToast);

  const [agents, setAgents] = useState<RemoteAgentConfig[]>([]);
  const [healthMap, setHealthMap] = useState<Record<string, RemoteAgentHealth>>({});
  const [loading, setLoading] = useState(false);
  const [showForm, setShowForm] = useState(false);

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const [list, health] = await Promise.all([
        client.listRemoteAgents(),
        client.healthCheckRemoteAgents(),
      ]);
      setAgents(list || []);
      setHealthMap(health || {});
    } catch (e) {
      console.error('加载远程 Agent 列表失败:', e);
    } finally {
      setLoading(false);
    }
  }, [client]);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const handleDisconnect = async (name: string) => {
    if (!window.confirm(t('remoteAgents.disconnectConfirm', { name }))) return;
    try {
      await client.disconnectRemoteAgent(name);
      showToast('success', t('remoteAgents.disconnectSuccess', { name }));
      await loadData();
    } catch (e) {
      const msg = e instanceof Error ? e.message : t('remoteAgents.disconnectFailed');
      showToast('error', msg);
    }
  };

  const handleRefreshHealth = async () => {
    try {
      const health = await client.healthCheckRemoteAgents();
      setHealthMap(health || {});
    } catch (e) {
      console.error('刷新健康状态失败:', e);
    }
  };

  return (
    <div className="p-6 max-w-5xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <div>
          <h2 className="text-lg font-semibold text-[var(--text-primary)]">{t('remoteAgents.title')}</h2>
          <p className="text-sm text-[var(--text-secondary)] mt-1">{t('remoteAgents.subtitle')}</p>
        </div>
        <button
          onClick={handleRefreshHealth}
          disabled={loading}
          className="px-4 py-2 bg-[var(--accent-600)] hover:bg-[var(--accent-700)] disabled:opacity-40 text-white text-sm rounded-lg transition-colors"
        >
          {t('remoteAgents.refreshHealth')}
        </button>
      </div>

      {/* 已连接 Agent 列表 */}
      {agents.length > 0 ? (
        <div className="space-y-4 mb-6">
          {agents.map((agent) => (
            <AgentCard
              key={agent.name}
              agent={agent}
              health={healthMap[agent.name]}
              onDisconnect={handleDisconnect}
            />
          ))}
        </div>
      ) : (
        <div className="mb-6 p-8 text-center bg-[var(--bg-card)] border border-[var(--border-color)] rounded-xl">
          <p className="text-sm text-[var(--text-secondary)]">{t('remoteAgents.noAgents')}</p>
          <p className="text-xs text-[var(--text-secondary)] mt-1">{t('remoteAgents.noAgentsHint')}</p>
        </div>
      )}

      {/* 添加 Agent 表单 */}
      <div className="bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl overflow-hidden shadow-sm">
        <button
          onClick={() => setShowForm(!showForm)}
          className="w-full px-5 py-4 text-left text-sm font-medium text-[var(--accent-600)] dark:text-[var(--accent-300)] hover:bg-[var(--bg-secondary)] transition-colors"
        >
          {showForm ? t('remoteAgents.hideForm') : t('remoteAgents.addAgent')}
        </button>
        {showForm && (
          <AddAgentForm
            onSuccess={() => {
              setShowForm(false);
              loadData();
            }}
          />
        )}
      </div>
    </div>
  );
}

/** Agent 卡片 */
function AgentCard({
  agent,
  health,
  onDisconnect,
}: {
  agent: RemoteAgentConfig;
  health?: RemoteAgentHealth;
  onDisconnect: (name: string) => void;
}) {
  const { t } = useTranslation();

  // 后端 AgentStatus 是 int: 0=stopped, 1=running, 2=error（无 MarshalJSON，序列化为数字）
  const normalizedStatus = (() => {
    if (!health) return 'stopped';
    const s = health.status;
    if (s === 1 || s === 'running') return 'running';
    if (s === 2 || s === 'error') return 'error';
    return 'stopped';
  })();

  const statusColor = (() => {
    switch (normalizedStatus) {
      case 'running': return 'bg-green-500';
      case 'error': return 'bg-red-500';
      default: return 'bg-[var(--text-secondary)]';
    }
  })();

  const statusText = (() => {
    switch (normalizedStatus) {
      case 'running': return t('remoteAgents.statusRunning');
      case 'error': return t('remoteAgents.statusError');
      default: return t('remoteAgents.statusStopped');
    }
  })();

  return (
    <div className="bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl overflow-hidden shadow-sm">
      <div className="px-5 py-4">
        <div className="flex items-center justify-between mb-2">
          <div className="flex items-center gap-3">
            <span className="text-sm font-medium text-[var(--text-primary)]">{agent.name}</span>
            <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${
              agent.transport === 'http'
                ? 'bg-[var(--accent-50)] text-[var(--accent-700)] dark:bg-[var(--accent-light)] dark:text-[var(--accent-300)]'
                : 'bg-[var(--bg-secondary)] text-[var(--text-secondary)]'
            }`}>
              {agent.transport}
            </span>
            <div className="flex items-center gap-1.5">
              <span className={`w-2 h-2 rounded-full ${statusColor}`} />
              <span className="text-xs text-[var(--text-secondary)]">{statusText}</span>
            </div>
          </div>
          <button
            onClick={() => onDisconnect(agent.name)}
            className="px-3 py-1 text-xs text-red-600 hover:bg-red-50 dark:hover:bg-red-900/20 rounded transition-colors"
          >
            {t('remoteAgents.disconnect')}
          </button>
        </div>
        {agent.description && (
          <p className="text-xs text-[var(--text-secondary)] mb-2">{agent.description}</p>
        )}
        {agent.skills && agent.skills.length > 0 && (
          <div className="flex flex-wrap gap-1">
            {agent.skills.map((skill) => (
              <span
                key={skill}
                className="text-xs px-2 py-0.5 bg-[var(--bg-secondary)] text-[var(--text-secondary)] rounded"
              >
                {skill}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

/** 添加 Agent 表单 */
function AddAgentForm({ onSuccess }: { onSuccess: () => void }) {
  const { t } = useTranslation();
  const client = useNodeClient();
  const showToast = useToastStore((s) => s.addToast);

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [transport, setTransport] = useState<'stdio' | 'http'>('http');
  const [command, setCommand] = useState('');
  const [args, setArgs] = useState('');
  const [url, setUrl] = useState('');
  const [skills, setSkills] = useState('');
  const [connecting, setConnecting] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const validate = (): boolean => {
    const errs: Record<string, string> = {};
    if (!name.trim()) errs.name = t('remoteAgents.nameRequired');
    if (transport === 'stdio' && !command.trim()) errs.command = t('remoteAgents.commandRequired');
    if (transport === 'http' && !url.trim()) errs.url = t('remoteAgents.urlRequired');
    setErrors(errs);
    return Object.keys(errs).length === 0;
  };

  const handleSubmit = async () => {
    if (!validate()) return;
    setConnecting(true);
    try {
      const cfg: RemoteAgentConfig = {
        name: name.trim(),
        description: description.trim(),
        transport,
        enabled: true,
        ...(transport === 'stdio'
          ? { command: command.trim(), args: args ? args.split(',').map((s) => s.trim()) : undefined }
          : { url: url.trim() }),
        ...(skills ? { skills: skills.split(',').map((s) => s.trim()) } : {}),
      };
      await client.connectRemoteAgent(cfg);
      showToast('success', t('remoteAgents.connectSuccess', { name: cfg.name }));
      onSuccess();
    } catch (e) {
      const msg = e instanceof Error ? e.message : t('remoteAgents.connectFailed');
      showToast('error', msg);
    } finally {
      setConnecting(false);
    }
  };

  const inputClass = 'w-full px-3 py-1.5 text-sm bg-[var(--bg-input)] border border-[var(--border-color)] rounded';

  return (
    <div className="p-5 space-y-4 bg-[var(--bg-primary)] border-t border-[var(--border-color)]">
      {/* 名称 */}
      <div>
        <label className="block text-sm text-[var(--text-secondary)] mb-1">
          {t('remoteAgents.name')} <span className="text-red-500">*</span>
        </label>
        <input
          type="text"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder={t('remoteAgents.namePlaceholder')}
          className={inputClass}
        />
        {errors.name && <p className="text-xs text-red-500 mt-1">{errors.name}</p>}
      </div>

      {/* 描述 */}
      <div>
        <label className="block text-sm text-[var(--text-secondary)] mb-1">{t('remoteAgents.description')}</label>
        <input
          type="text"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder={t('remoteAgents.descriptionPlaceholder')}
          className={inputClass}
        />
      </div>

      {/* 传输类型 */}
      <div>
        <label className="block text-sm text-[var(--text-secondary)] mb-1">{t('remoteAgents.transport')}</label>
        <select
          value={transport}
          onChange={(e) => setTransport(e.target.value as 'stdio' | 'http')}
          className={inputClass}
        >
          <option value="http">http</option>
          <option value="stdio">stdio</option>
        </select>
      </div>

      {/* stdio 字段 */}
      {transport === 'stdio' && (
        <>
          <div>
            <label className="block text-sm text-[var(--text-secondary)] mb-1">
              {t('remoteAgents.command')} <span className="text-red-500">*</span>
            </label>
            <input
              type="text"
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              placeholder={t('remoteAgents.commandPlaceholder')}
              className={inputClass}
            />
            {errors.command && <p className="text-xs text-red-500 mt-1">{errors.command}</p>}
          </div>
          <div>
            <label className="block text-sm text-[var(--text-secondary)] mb-1">{t('remoteAgents.args')}</label>
            <input
              type="text"
              value={args}
              onChange={(e) => setArgs(e.target.value)}
              placeholder={t('remoteAgents.argsPlaceholder')}
              className={inputClass}
            />
          </div>
        </>
      )}

      {/* http 字段 */}
      {transport === 'http' && (
        <div>
          <label className="block text-sm text-[var(--text-secondary)] mb-1">
            {t('remoteAgents.url')} <span className="text-red-500">*</span>
          </label>
          <input
            type="text"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder={t('remoteAgents.urlPlaceholder')}
            className={inputClass}
          />
          {errors.url && <p className="text-xs text-red-500 mt-1">{errors.url}</p>}
        </div>
      )}

      {/* 能力标签 */}
      <div>
        <label className="block text-sm text-[var(--text-secondary)] mb-1">{t('remoteAgents.skills')}</label>
        <input
          type="text"
          value={skills}
          onChange={(e) => setSkills(e.target.value)}
          placeholder={t('remoteAgents.skillsPlaceholder')}
          className={inputClass}
        />
      </div>

      {/* 提交 */}
      <div className="pt-2">
        <button
          onClick={handleSubmit}
          disabled={connecting}
          className="px-4 py-2 text-sm bg-[var(--accent-600)] hover:bg-[var(--accent-700)] disabled:opacity-40 text-white rounded transition-colors"
        >
          {connecting ? t('remoteAgents.connecting') : t('remoteAgents.connect')}
        </button>
      </div>
    </div>
  );
}
