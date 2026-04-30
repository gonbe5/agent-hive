import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { X } from 'lucide-react';

interface TagEditorProps {
  sessionId: string;
  initialTags: string[];
  onSave: (tags: string[]) => Promise<void>;
  onClose: () => void;
}

export function TagEditor({ initialTags, onSave, onClose }: TagEditorProps) {
  const { t } = useTranslation();
  const [tags, setTags] = useState<string[]>(initialTags);
  const [input, setInput] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const triggerRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    // 记录触发元素，关闭时恢复焦点
    triggerRef.current = document.activeElement as HTMLElement;
    // 打开时 focus 输入框
    setTimeout(() => inputRef.current?.focus(), 0);
    // Escape 关闭
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') { onClose(); }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      triggerRef.current?.focus();
    };
  }, [onClose]);

  const addTag = () => {
    const trimmed = input.trim();
    if (!trimmed) return;
    if (tags.includes(trimmed)) { setInput(''); return; }
    if (tags.length >= 10) { setError('标签数量不能超过 10 个'); return; }
    if ([...trimmed].length > 50) { setError('单个标签长度不能超过 50 字符'); return; }
    setTags([...tags, trimmed]);
    setInput('');
    setError(null);
  };

  const removeTag = (index: number) => {
    setTags(tags.filter((_, i) => i !== index));
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      addTag();
    }
    if (e.key === 'Backspace' && !input && tags.length > 0) {
      removeTag(tags.length - 1);
    }
  };

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    try {
      await onSave(tags);
      onClose();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : '保存失败');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm">
      <div
        role="dialog"
        aria-modal="true"
        aria-label={t('tags.edit', '编辑标签')}
        className="w-96 max-w-[90vw] rounded-xl bg-[var(--bg-card)] border border-[var(--border-color)] shadow-2xl"
      >
        {/* Header */}
        <div className="flex items-center justify-between px-5 py-4 border-b border-[var(--border-color)]">
          <h2 className="text-sm font-semibold text-[var(--text-primary)]">
            {t('tags.edit', '编辑标签')}
          </h2>
          <button
            onClick={onClose}
            className="p-1 rounded-md text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-secondary)] transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Body */}
        <div className="px-5 py-4 space-y-3">
          {error && (
            <div className="px-3 py-2 rounded-md bg-red-50 dark:bg-red-900/20 text-red-600 dark:text-red-400 text-xs">
              {error}
            </div>
          )}

          <div className="min-h-[44px] flex flex-wrap gap-1.5 p-2 rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] focus-within:ring-2 focus-within:ring-[var(--accent-subtle)] focus-within:border-[var(--accent)] transition-all">
            {tags.map((tag, i) => (
              <span
                key={i}
                className="inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-[var(--accent-100)] dark:bg-[var(--accent-light)] text-[var(--accent-700)] dark:text-[var(--accent-300)] text-xs font-medium"
              >
                {tag}
                <button
                  onClick={() => removeTag(i)}
                  className="ml-0.5 rounded-full hover:bg-[var(--accent-100)] dark:hover:bg-[var(--accent-light)] transition-colors"
                >
                  <X className="w-2.5 h-2.5" />
                </button>
              </span>
            ))}
            {tags.length < 10 && (
              <input
                ref={inputRef}
                type="text"
                value={input}
                onChange={(e) => { setInput(e.target.value); setError(null); }}
                onKeyDown={handleKeyDown}
                onBlur={addTag}
                placeholder={tags.length === 0 ? t('tags.placeholder', '输入标签，回车添加') : ''}
                className="flex-1 min-w-[80px] text-xs bg-transparent outline-none text-[var(--text-primary)] placeholder:text-[var(--text-secondary)]"
              />
            )}
          </div>

          <p className="text-xs text-[var(--text-secondary)]">
            {t('tags.hint', '最多 10 个标签，每个标签最多 50 字符（支持中文）')}
          </p>
        </div>

        {/* Footer */}
        <div className="flex items-center justify-end gap-2 px-5 py-3 border-t border-[var(--border-color)]">
          <button
            onClick={onClose}
            className="px-4 py-1.5 text-xs rounded-lg border border-[var(--border-color)] text-[var(--text-secondary)] hover:bg-[var(--bg-secondary)] transition-colors"
          >
            {t('common.cancel', '取消')}
          </button>
          <button
            onClick={handleSave}
            disabled={saving}
            className="px-4 py-1.5 text-xs rounded-lg bg-[var(--accent-500)] hover:bg-[var(--accent-600)] text-white transition-colors disabled:opacity-50"
          >
            {saving ? t('common.saving', '保存中...') : t('common.save', '保存')}
          </button>
        </div>
      </div>
    </div>
  );
}
