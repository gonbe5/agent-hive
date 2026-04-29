import { useEffect, useState, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Zap, Plus, Pencil, Trash2, X, Save, RotateCcw } from 'lucide-react';
import { useNodeClient } from '../hooks/useNodeClient';
import { useToastStore } from '../store/toast';
import type { AdminSkillItem, AdminSkillDetail } from '../types/api';

// ── 编辑 Modal ────────────────────────────────────────────────────────────────

interface SkillModalProps {
  initial?: AdminSkillDetail | null;
  onSave: (name: string, content: string, revision: number) => Promise<void>;
  onClose: () => void;
}

function SkillModal({ initial, onSave, onClose }: SkillModalProps) {
  const { t } = useTranslation();
  const [name, setName] = useState(initial?.name ?? '');
  const [content, setContent] = useState(initial?.content ?? DEFAULT_TEMPLATE);
  const [saving, setSaving] = useState(false);
  const isEdit = !!initial;

  // 新建时：name 输入框变化时自动同步 frontmatter 中的 name 字段
  const handleNameChange = (newName: string) => {
    setName(newName);
    if (!isEdit) {
      setContent(prev => prev.replace(/^(name:\s*).+$/m, `$1${newName || 'my-skill'}`));
    }
  };

  const handleSave = async () => {
    if (!name.trim() || !content.trim()) return;
    setSaving(true);
    try {
      await onSave(name.trim(), content, initial?.revision ?? 0);
      onClose();
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm">
      <div className="bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl shadow-2xl w-full max-w-3xl mx-4 flex flex-col max-h-[90vh]">
        {/* 标题栏 */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--border-color)]">
          <h3 className="text-sm font-semibold text-[var(--text-primary)] font-display">
            {isEdit ? t('skills.editSkill') : t('skills.newSkill')}
          </h3>
          <button onClick={onClose} className="text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors">
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* 名称 */}
        {!isEdit && (
          <div className="px-5 pt-4">
            <label className="block text-xs font-medium text-[var(--text-secondary)] mb-1">{t('skills.skillName')}</label>
            <input
              type="text"
              className="w-full text-sm border border-[var(--border-color)] rounded-lg px-3 py-2 bg-[var(--bg-secondary)] text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-500)]/40 font-mono"
              placeholder="my-skill"
              value={name}
              onChange={(e) => handleNameChange(e.target.value)}
            />
          </div>
        )}

        {/* 内容编辑器 */}
        <div className="flex-1 px-5 pt-4 pb-4 flex flex-col min-h-0">
          <label className="block text-xs font-medium text-[var(--text-secondary)] mb-1">{t('skills.content')}</label>
          <textarea
            className="flex-1 min-h-[320px] w-full text-xs font-mono border border-[var(--border-color)] rounded-lg px-3 py-2 bg-[var(--bg-secondary)] text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-500)]/40 resize-none"
            spellCheck={false}
            value={content}
            onChange={(e) => setContent(e.target.value)}
          />
        </div>

        {/* 操作按钮 */}
        <div className="flex justify-end gap-2 px-5 pb-4">
          <button
            onClick={onClose}
            className="px-4 py-2 text-xs rounded-lg border border-[var(--border-color)] text-[var(--text-secondary)] hover:bg-[var(--bg-secondary)] transition-colors"
          >
            {t('common.cancel')}
          </button>
          <button
            onClick={handleSave}
            disabled={saving || !name.trim() || !content.trim()}
            className="flex items-center gap-1.5 px-4 py-2 text-xs rounded-lg bg-[var(--accent-500)] text-white hover:bg-[var(--accent-600)] disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            <Save className="w-3.5 h-3.5" />
            {saving ? t('common.saving') : t('common.save')}
          </button>
        </div>
      </div>
    </div>
  );
}

// ── 默认模板 ──────────────────────────────────────────────────────────────────

const DEFAULT_TEMPLATE = `---
name: my-skill
description: 简短描述这个 skill 的作用
---

在这里编写 skill 的详细指令和知识内容。

## 使用说明

- 步骤一
- 步骤二
`;

// ── 主页面 ────────────────────────────────────────────────────────────────────

export function Skills() {
  const { t } = useTranslation();
  const client = useNodeClient();
  const addToast = useToastStore((s) => s.addToast);

  const [skills, setSkills] = useState<AdminSkillItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [modalSkill, setModalSkill] = useState<AdminSkillDetail | null | undefined>(undefined); // undefined=关闭, null=新建, Detail=编辑
  const [deleting, setDeleting] = useState<string | null>(null);
  const loadRef = useRef(0);

  const load = async () => {
    const seq = ++loadRef.current;
    setLoading(true);
    setError(null);
    try {
      const res = await client.adminListSkills();
      if (seq === loadRef.current) setSkills(res.items);
    } catch (e) {
      const msg = e instanceof Error ? e.message : t('skills.loadError');
      if (seq === loadRef.current) {
        setError(msg);
        addToast('error', msg);
      }
    } finally {
      if (seq === loadRef.current) setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  const openNew = () => setModalSkill(null);

  const openEdit = async (name: string) => {
    try {
      const detail = await client.adminGetSkill(name);
      setModalSkill(detail);
    } catch (e) {
      addToast('error', e instanceof Error ? e.message : t('skills.loadError'));
    }
  };

  const handleSave = async (name: string, content: string, revision: number) => {
    await client.adminUpsertSkill(name, content, revision);
    addToast('success', t('skills.saved'));
    await load();
  };

  const handleDelete = async (name: string) => {
    if (!window.confirm(t('skills.confirmDelete', { name }))) return;
    setDeleting(name);
    try {
      await client.adminDeleteSkill(name);
      addToast('success', t('skills.deleted'));
      await load();
    } catch (e) {
      addToast('error', e instanceof Error ? e.message : t('skills.deleteError'));
    } finally {
      setDeleting(null);
    }
  };

  return (
    <div className="p-6 max-w-5xl mx-auto">
      {/* 标题行 */}
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-lg font-semibold text-[var(--text-primary)] font-display">{t('skills.title')}</h2>
        <div className="flex items-center gap-2">
          <button
            onClick={load}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg border border-[var(--border-color)] text-[var(--text-secondary)] hover:bg-[var(--bg-secondary)] transition-colors"
            title={t('common.refresh')}
          >
            <RotateCcw className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={openNew}
            className="flex items-center gap-1.5 px-3 py-1.5 text-xs rounded-lg bg-[var(--accent-500)] text-white hover:bg-[var(--accent-600)] transition-colors"
          >
            <Plus className="w-3.5 h-3.5" />
            {t('skills.newSkill')}
          </button>
        </div>
      </div>

      {/* 内容区 */}
      {loading ? (
        <div className="text-center py-12 text-[var(--text-secondary)] text-sm animate-pulse">{t('skills.loading')}</div>
      ) : error ? (
        <div className="text-center py-20">
          <Zap className="w-16 h-16 mx-auto mb-4 text-red-300 dark:text-red-700" />
          <div className="text-red-500 text-sm">{error}</div>
        </div>
      ) : skills.length === 0 ? (
        <div className="text-center py-20">
          <Zap className="w-16 h-16 mx-auto mb-4 text-[var(--text-secondary)] opacity-30" />
          <div className="text-[var(--text-secondary)] text-sm">{t('skills.noSkills')}</div>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3">
          {skills.map((skill) => (
            <div
              key={skill.name}
              className="bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl shadow-sm p-5 flex flex-col gap-2"
            >
              {/* 名称 + 来源标记 */}
              <div className="flex items-center gap-2">
                <Zap className="w-4 h-4 text-[var(--accent-600)] dark:text-[var(--accent-300)] shrink-0" />
                <span className="text-sm font-medium text-[var(--accent-600)] dark:text-[var(--accent-300)] truncate">{skill.name}</span>
                <span className={`ml-auto shrink-0 text-[10px] px-1.5 py-0.5 rounded-md font-mono uppercase ${
                  skill.origin === 'db'
                    ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/20 dark:text-blue-400'
                    : 'bg-[var(--bg-secondary)] text-[var(--text-secondary)]'
                }`}>
                  {skill.origin}
                </span>
              </div>

              {/* 描述 */}
              <p className="text-xs text-[var(--text-secondary)] line-clamp-3 flex-1">{skill.description || '—'}</p>

              {/* 路径 */}
              {skill.path && (
                <p className="text-[10px] font-mono text-[var(--text-secondary)] opacity-60 truncate">{skill.path}</p>
              )}

              {/* 操作按钮 */}
              <div className="flex items-center gap-2 pt-1 border-t border-[var(--border-color)]">
                <button
                  onClick={() => openEdit(skill.name)}
                  className="flex items-center gap-1 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
                >
                  <Pencil className="w-3 h-3" />
                  {t('common.edit')}
                </button>
                {skill.origin === 'db' && (
                  <button
                    onClick={() => handleDelete(skill.name)}
                    disabled={deleting === skill.name}
                    className="flex items-center gap-1 text-xs text-red-500 hover:text-red-600 disabled:opacity-50 transition-colors ml-auto"
                  >
                    <Trash2 className="w-3 h-3" />
                    {deleting === skill.name ? t('common.deleting') : t('common.delete')}
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* 编辑/新建 Modal */}
      {modalSkill !== undefined && (
        <SkillModal
          initial={modalSkill}
          onSave={handleSave}
          onClose={() => setModalSkill(undefined)}
        />
      )}
    </div>
  );
}
