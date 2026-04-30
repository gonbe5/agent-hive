import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNodeClient } from '../../hooks/useNodeClient';
import type { WeChatConfigResponse, WeChatProtocolStatus } from '../../types/api';
import { useToastStore } from '../../store/toast';

export function WeChatSettings() {
  const { t } = useTranslation();
  const client = useNodeClient();
  const [config, setConfig] = useState<WeChatConfigResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const showToast = useToastStore((s) => s.addToast);

  const loadConfig = useCallback(async () => {
    try {
      const data = await client.getWeChatConfig();
      setConfig(data);
    } catch (error) {
      console.error('Failed to load WeChat config:', error);
      showToast('error', t('runtimeConfig.loadFailed'));
    }
  }, [client, showToast, t]);

  useEffect(() => {
    loadConfig();
  }, [loadConfig]);

  const handleUpdate = async (protocol: string, enabled: boolean, protocolConfig: Record<string, unknown>) => {
    setLoading(true);
    try {
      await client.updateWeChatProtocol(protocol, { enabled, config: protocolConfig });
      await loadConfig();
      showToast('success', t('runtimeConfig.applySuccess'));
    } catch (error) {
      console.error('Failed to update config:', error);
      showToast('error', t('runtimeConfig.applyFailed'));
    } finally {
      setLoading(false);
    }
  };

  const handleSave = async () => {
    setLoading(true);
    try {
      const result = await client.saveConfig();
      showToast('success', result.message);
    } catch (error) {
      console.error('Failed to save config:', error);
      showToast('error', t('runtimeConfig.saveFailed'));
    } finally {
      setLoading(false);
    }
  };

  const handleReload = async (protocol: string) => {
    setLoading(true);
    try {
      const result = await client.reloadWeChatProtocol(protocol);
      showToast('success', result.message);
      await loadConfig();
    } catch (error) {
      console.error('Failed to reload protocol:', error);
      showToast('error', t('runtimeConfig.applyFailed'));
    } finally {
      setLoading(false);
    }
  };

  if (!config) {
    return <div className="p-6">{t('common.loading')}</div>;
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold text-[var(--text-primary)] font-display">{t('runtimeConfig.wechatTitle')}</h3>
          <p className="text-xs text-[var(--text-secondary)] mt-0.5">
            {t('runtimeConfig.wechatDesc')}
          </p>
        </div>
        <button
          onClick={handleSave}
          disabled={loading}
          className="px-4 py-2 text-sm bg-[var(--accent-600)] hover:bg-[var(--accent-700)] disabled:opacity-50 text-white rounded-lg transition-colors"
        >
          {loading ? t('runtimeConfig.saving') : t('runtimeConfig.saveConfig')}
        </button>
      </div>

      <div className="space-y-3">
        <ProtocolCard
          title="WeChatPadPro (iPad)"
          protocol="wechatpadpro"
          status={config.protocols.wechatpadpro}
          onUpdate={handleUpdate}
          onReload={handleReload}
          loading={loading}
          badge={{ text: t('runtimeConfig.recommended'), color: 'green' }}
          description={t('runtimeConfig.wechatpadproDesc')}
        />

        <ProtocolCard
          title="Wechaty (gRPC)"
          protocol="wechaty"
          status={config.protocols.wechaty}
          onUpdate={handleUpdate}
          onReload={handleReload}
          loading={loading}
          badge={{ text: t('runtimeConfig.alternative'), color: 'yellow' }}
          description={t('runtimeConfig.wechatyDesc')}
        />
      </div>

      <div className="p-4 bg-[var(--accent-50)] dark:bg-[var(--accent-light)] border border-[var(--accent-border)] dark:border-[var(--accent-border)] rounded-lg">
        <h4 className="text-sm font-medium text-[var(--accent-700)] dark:text-[var(--accent-100)] mb-2">{t('runtimeConfig.usageTips')}</h4>
        <ul className="text-sm text-[var(--accent-700)] dark:text-[var(--accent-300)] space-y-1 list-disc list-inside">
          <li><strong>WeChatPadPro</strong>: {t('runtimeConfig.wechatTip1')}</li>
          <li><strong>Wechaty</strong>: {t('runtimeConfig.wechatTip2')}</li>
          <li>{t('runtimeConfig.wechatTip3')}</li>
          <li>{t('runtimeConfig.wechatTip4')}</li>
        </ul>
      </div>
    </div>
  );
}

/** 协议卡片组件 */
interface ProtocolCardProps {
  title: string;
  protocol: string;
  status: WeChatProtocolStatus;
  onUpdate: (protocol: string, enabled: boolean, config: Record<string, unknown>) => void;
  onReload: (protocol: string) => void;
  loading: boolean;
  badge?: { text: string; color: 'green' | 'red' | 'yellow' };
  description?: string;
}

function ProtocolCard({ title, protocol, status, onUpdate, onReload, loading, badge, description }: ProtocolCardProps) {
  const { t } = useTranslation();
  const [enabled, setEnabled] = useState(status.enabled);
  const [config, setConfig] = useState<Record<string, unknown>>(status.config);
  const [expanded, setExpanded] = useState(false);

  useEffect(() => {
    setEnabled(status.enabled);
    setConfig(status.config);
  }, [status]);

  const handleSave = () => {
    onUpdate(protocol, enabled, config);
  };

  const getStatusColor = () => {
    if (!enabled) return 'text-[var(--text-secondary)]';
    switch (status.status) {
      case 'connected':
        return status.logged_in ? 'text-green-500' : 'text-yellow-500';
      case 'error':
        return 'text-red-500';
      default:
        return 'text-[var(--text-secondary)]';
    }
  };

  const getStatusText = () => {
    if (!enabled) return t('runtimeConfig.notEnabled');
    switch (status.status) {
      case 'connected':
        return status.logged_in ? t('runtimeConfig.connectedLoggedIn') : t('runtimeConfig.connectedNotLoggedIn');
      case 'error':
        return t('runtimeConfig.statusError');
      default:
        return t('runtimeConfig.notStarted');
    }
  };

  return (
    <div className="bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl overflow-hidden shadow-sm">
      <div className="px-5 py-4 border-b border-[var(--border-color)]">
        <div className="flex items-center justify-between mb-2">
          <div className="flex items-center gap-3">
            <span className="text-sm font-medium text-[var(--text-primary)]">{title}</span>
            {badge && (
              <span className={`text-xs px-2 py-0.5 rounded-full font-medium ${
                badge.color === 'green' ? 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300' :
                badge.color === 'red' ? 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300' :
                'bg-yellow-100 text-yellow-700 dark:bg-yellow-900 dark:text-yellow-300'
              }`}>
                {badge.text}
              </span>
            )}
            <span className={`text-xs font-medium ${getStatusColor()}`}>{getStatusText()}</span>
          </div>
          <div className="flex items-center gap-2">
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
                className="w-4 h-4 accent-[var(--accent-600)]"
              />
              <span className="text-sm text-[var(--text-secondary)]">{t('common.enable')}</span>
            </label>
            <button
              onClick={() => setExpanded(!expanded)}
              className="px-2 py-1 text-xs text-[var(--accent-600)] hover:text-[var(--accent-700)] dark:text-[var(--accent-300)]"
            >
              {expanded ? `${t('common.collapse')} ▲` : `${t('common.expand')} ▼`}
            </button>
          </div>
        </div>
        {description && (
          <p className="text-xs text-[var(--text-secondary)]">{description}</p>
        )}
      </div>

      {expanded && (
        <div className="p-5 space-y-4 bg-[var(--bg-primary)]">
          <ConfigFields protocol={protocol} config={config} onChange={setConfig} />
          <div className="flex gap-2 pt-2">
            <button
              onClick={handleSave}
              disabled={loading}
              className="px-3 py-1.5 text-sm bg-[var(--accent-600)] hover:bg-[var(--accent-700)] disabled:opacity-50 text-white rounded transition-colors"
            >
              {t('common.save')}
            </button>
            {status.status === 'connected' && (
              <button
                onClick={() => onReload(protocol)}
                disabled={loading}
                className="px-3 py-1.5 text-sm bg-orange-600 hover:bg-orange-700 disabled:opacity-50 text-white rounded transition-colors"
              >
                {t('runtimeConfig.reload')}
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

/** 配置字段组件 */
interface ConfigFieldsProps {
  protocol: string;
  config: Record<string, unknown>;
  onChange: (config: Record<string, unknown>) => void;
}

function ConfigFields({ protocol, config, onChange }: ConfigFieldsProps) {
  const { t } = useTranslation();
  const updateField = (key: string, value: unknown) => {
    onChange({ ...config, [key]: value });
  };

  switch (protocol) {
    case 'wechatpadpro':
      return (
        <>
          <div>
            <label className="block text-sm text-[var(--text-secondary)] mb-1">
              Base URL <span className="text-red-500">*</span>
            </label>
            <input
              type="text"
              value={(config.base_url as string) || ''}
              onChange={(e) => updateField('base_url', e.target.value)}
              placeholder="http://localhost:1238"
              className="w-full px-3 py-1.5 text-sm bg-[var(--bg-input)] border border-[var(--border-color)] rounded focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)] focus:border-[var(--accent)]"
            />
            <p className="text-xs text-[var(--text-secondary)] mt-1">
              {t('runtimeConfig.wechatpadproBaseUrlHint')}
            </p>
          </div>
          <div>
            <label className="block text-sm text-[var(--text-secondary)] mb-1">App ID</label>
            <input
              type="text"
              value={(config.app_id as string) || ''}
              onChange={(e) => updateField('app_id', e.target.value)}
              placeholder={t('common.optional')}
              className="w-full px-3 py-1.5 text-sm bg-[var(--bg-input)] border border-[var(--border-color)] rounded focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)] focus:border-[var(--accent)]"
            />
          </div>
          <div>
            <label className="block text-sm text-[var(--text-secondary)] mb-1">
              Token <span className="text-red-500">*</span>
            </label>
            <input
              type="password"
              value={(config.token as string) || ''}
              onChange={(e) => updateField('token', e.target.value)}
              placeholder={t('runtimeConfig.wechatpadproTokenHint')}
              className="w-full px-3 py-1.5 text-sm bg-[var(--bg-input)] border border-[var(--border-color)] rounded focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)] focus:border-[var(--accent)]"
            />
            <p className="text-xs text-[var(--text-secondary)] mt-1">
              {t('runtimeConfig.wechatpadproAdminKeyHint')}
            </p>
          </div>
          <div>
            <label className="block text-sm text-[var(--text-secondary)] mb-1">{t('runtimeConfig.wechatpadproTimeout')}</label>
            <input
              type="number"
              value={(config.timeout as number) || 30}
              onChange={(e) => updateField('timeout', parseInt(e.target.value) || 30)}
              min="5"
              max="300"
              className="w-full px-3 py-1.5 text-sm bg-[var(--bg-input)] border border-[var(--border-color)] rounded focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)] focus:border-[var(--accent)]"
            />
          </div>
        </>
      );

    case 'wechaty':
      return (
        <>
          <div>
            <label className="block text-sm text-[var(--text-secondary)] mb-1">
              Endpoint <span className="text-red-500">*</span>
            </label>
            <input
              type="text"
              value={(config.endpoint as string) || ''}
              onChange={(e) => updateField('endpoint', e.target.value)}
              placeholder="localhost:8788"
              className="w-full px-3 py-1.5 text-sm bg-[var(--bg-input)] border border-[var(--border-color)] rounded focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)] focus:border-[var(--accent)]"
            />
            <p className="text-xs text-[var(--text-secondary)] mt-1">
              {t('runtimeConfig.wechatyEndpointHint')}
            </p>
          </div>
          <div>
            <label className="block text-sm text-[var(--text-secondary)] mb-1">Token</label>
            <input
              type="password"
              value={(config.token as string) || ''}
              onChange={(e) => updateField('token', e.target.value)}
              placeholder={t('runtimeConfig.wechatyTokenPlaceholder')}
              className="w-full px-3 py-1.5 text-sm bg-[var(--bg-input)] border border-[var(--border-color)] rounded focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)] focus:border-[var(--accent)]"
            />
          </div>
        </>
      );

    default:
      return <div className="text-sm text-[var(--text-secondary)]">{t('runtimeConfig.noConfigItems')}</div>;
  }
}
