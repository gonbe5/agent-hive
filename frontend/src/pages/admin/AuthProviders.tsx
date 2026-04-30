import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Plus, Trash2, Check, X } from 'lucide-react';
import { useNodeClient } from '../../hooks/useNodeClient';
import { useToastStore } from '../../store/toast';
import type { AdminProvider } from '../../types/api';

const PROVIDER_TYPES = ['feishu', 'dingtalk', 'ldap', 'wecom'];

export function AuthProviders() {
  const { t } = useTranslation();
  const client = useNodeClient();
  const addToast = useToastStore((s) => s.addToast);
  const [providers, setProviders] = useState<AdminProvider[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [newName, setNewName] = useState('');
  const [newType, setNewType] = useState('feishu');
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const list = await client.adminListProviders();
      setProviders(list);
    } catch (e: unknown) {
      addToast('error', e instanceof Error ? e.message : '加载 Provider 列表失败');
    } finally {
      setLoading(false);
    }
  }, [client, addToast]);

  useEffect(() => { load(); }, [load]);

  const handleCreate = async () => {
    if (!newName.trim()) { addToast('error', 'Provider 名称不能为空'); return; }
    try {
      await client.adminCreateProvider({ name: newName.trim(), provider_type: newType, enabled: false, config_json: {} });
      addToast('success', `Provider "${newName}" 已创建`);
      setNewName('');
      setCreating(false);
      load();
    } catch (e: unknown) {
      addToast('error', e instanceof Error ? e.message : '创建 Provider 失败');
    }
  };

  const handleToggleEnabled = async (p: AdminProvider) => {
    try {
      await client.adminUpdateProvider(p.name, { enabled: !p.enabled });
      addToast('success', `Provider "${p.name}" 已${!p.enabled ? '启用' : '禁用'}`);
      load();
    } catch (e: unknown) {
      addToast('error', e instanceof Error ? e.message : '更新 Provider 失败');
    }
  };

  const handleDelete = async (name: string) => {
    setDeleteConfirm(name);
  };

  const handleDeleteConfirmed = async () => {
    if (!deleteConfirm) return;
    const name = deleteConfirm;
    setDeleteConfirm(null);
    try {
      await client.adminDeleteProvider(name);
      addToast('success', `Provider "${name}" 已删除`);
      load();
    } catch (e: unknown) {
      addToast('error', e instanceof Error ? e.message : '删除 Provider 失败');
    }
  };

  return (
    <div className="p-6 max-w-4xl mx-auto">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-[var(--text-primary)]">{t('admin.authProviders', '认证 Provider')}</h1>
          <p className="text-sm text-[var(--text-secondary)] mt-1">{t('admin.authProvidersDesc', '管理飞书、钉钉、LDAP 等登录方式')}</p>
        </div>
        <button
          onClick={() => setCreating(true)}
          className="flex items-center gap-2 px-3 py-2 text-sm rounded-lg bg-[var(--accent-500)] hover:bg-[var(--accent-600)] text-white transition-colors"
        >
          <Plus className="w-4 h-4" />
          {t('admin.addProvider', '添加 Provider')}
        </button>
      </div>

      {creating && (
        <div className="mb-4 p-4 rounded-xl border border-[var(--border-color)] bg-[var(--bg-card)]">
          <h3 className="text-sm font-medium text-[var(--text-primary)] mb-3">{t('admin.newProvider', '新建 Provider')}</h3>
          <div className="flex items-center gap-3">
            <input
              type="text"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder={t('admin.providerName', 'Provider 名称（唯一标识）')}
              className="flex-1 px-3 py-2 text-sm rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)]"
              onKeyDown={(e) => { if (e.key === 'Enter') handleCreate(); if (e.key === 'Escape') setCreating(false); }}
              autoFocus
            />
            <select
              value={newType}
              onChange={(e) => setNewType(e.target.value)}
              className="px-3 py-2 text-sm rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)]"
            >
              {PROVIDER_TYPES.map((pt) => <option key={pt} value={pt}>{pt}</option>)}
            </select>
            <button onClick={handleCreate} className="p-2 rounded-lg bg-green-500 hover:bg-green-600 text-white transition-colors">
              <Check className="w-4 h-4" />
            </button>
            <button onClick={() => setCreating(false)} className="p-2 rounded-lg border border-[var(--border-color)] text-[var(--text-secondary)] hover:bg-[var(--bg-secondary)] transition-colors">
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>
      )}

      {loading ? (
        <div className="text-center py-12 text-[var(--text-secondary)] text-sm animate-pulse">{t('common.loading', '加载中...')}</div>
      ) : providers.length === 0 ? (
        <div className="text-center py-12 text-[var(--text-secondary)] text-sm">
          {t('admin.noProviders', '暂无 Provider，点击右上角添加')}
        </div>
      ) : (
        <div className="space-y-3">
          {providers.map((p) => (
            <div key={p.name} className="flex items-center justify-between p-4 rounded-xl border border-[var(--border-color)] bg-[var(--bg-card)]">
              <div className="flex items-center gap-4">
                <div>
                  <div className="font-medium text-[var(--text-primary)]">{p.name}</div>
                  <div className="text-xs text-[var(--text-secondary)] mt-0.5">{p.provider_type}</div>
                </div>
                <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${
                  p.enabled
                    ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                    : 'bg-[var(--bg-secondary)] text-[var(--text-secondary)]'
                }`}>
                  {p.enabled ? t('admin.enabled', '已启用') : t('admin.disabled', '已禁用')}
                </span>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => handleToggleEnabled(p)}
                  className={`text-xs px-3 py-1.5 rounded-lg border transition-colors ${
                    p.enabled
                      ? 'border-[var(--border-color)] text-[var(--text-secondary)] hover:bg-[var(--bg-secondary)]'
                      : 'border-green-200 text-green-600 hover:bg-green-50 dark:border-green-800 dark:hover:bg-green-900/20'
                  }`}
                >
                  {p.enabled ? t('admin.disable', '禁用') : t('admin.enable', '启用')}
                </button>
                <button
                  onClick={() => handleDelete(p.name)}
                  className="p-1.5 rounded-lg text-[var(--text-secondary)] hover:text-red-500 hover:bg-red-50 dark:hover:bg-red-900/20 transition-colors"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* 删除确认对话框 */}
      {deleteConfirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm">
          <div className="w-80 rounded-xl bg-[var(--bg-card)] border border-[var(--border-color)] shadow-2xl p-6">
            <h3 className="text-sm font-semibold text-[var(--text-primary)] mb-2">删除 Provider</h3>
            <p className="text-sm text-[var(--text-secondary)] mb-5">
              确定删除 <span className="font-medium text-[var(--text-primary)]">{deleteConfirm}</span>？此操作不可撤销。
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setDeleteConfirm(null)}
                className="px-3 py-1.5 text-xs rounded-lg border border-[var(--border-color)] text-[var(--text-secondary)] hover:bg-[var(--bg-secondary)] transition-colors"
              >
                取消
              </button>
              <button
                onClick={handleDeleteConfirmed}
                className="px-3 py-1.5 text-xs rounded-lg bg-red-600 hover:bg-red-700 text-white transition-colors"
              >
                删除
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
