import { useState, useRef, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Globe } from 'lucide-react';
import { useLanguage } from '../../hooks/useLanguage';

/** 语言切换下拉组件 */
export function LanguageSwitcher() {
  const { t } = useTranslation();
  const { language, setLanguage } = useLanguage();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  // 点击外部关闭下拉
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, []);

  // Escape 键关闭下拉
  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [open]);

  const options = [
    { code: 'zh' as const, label: '中文' },
    { code: 'en' as const, label: 'English' },
  ];

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1.5 px-2.5 py-2.5 rounded-lg text-sm text-[var(--text-secondary)] hover:text-[var(--text-primary)] hover:bg-[var(--bg-hover)] transition-colors min-h-[44px]"
        title={t('header.switchLanguage')}
        aria-label={t('header.switchLanguage')}
      >
        <Globe className="w-4 h-4" />
        <span className="text-xs font-medium">{language === 'zh' ? '中' : 'EN'}</span>
      </button>

      {open && (
        <div className="absolute right-0 top-full mt-1 w-32 bg-[var(--bg-card)] border border-[var(--border-color)] rounded-xl shadow-lg overflow-hidden z-50">
          {options.map((opt) => (
            <button
              key={opt.code}
              onClick={() => {
                setLanguage(opt.code);
                setOpen(false);
              }}
              className={`w-full text-left px-3 py-2 text-sm transition-colors ${
                language === opt.code
                  ? 'bg-[var(--accent-50)] dark:bg-[var(--accent-light)] text-[var(--accent-600)] dark:text-[var(--accent-300)] font-medium'
                  : 'text-[var(--text-primary)] hover:bg-[var(--bg-hover)]'
              }`}
            >
              {opt.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
