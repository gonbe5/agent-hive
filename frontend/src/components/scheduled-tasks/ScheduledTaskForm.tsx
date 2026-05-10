import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Check, X } from 'lucide-react';
import type { ScheduledTask, ScheduledTaskTargetType, ScheduledTaskUpsertRequest } from '../../types/api';

type ScheduleMode = 'interval' | 'cron';

interface FormState {
  name: string;
  description: string;
  targetType: ScheduledTaskTargetType;
  targetConfigJson: string;
  platform: string;
  prompt: string;
  scheduleMode: ScheduleMode;
  intervalSec: string;
  cronExpr: string;
  timezone: string;
  enabled: boolean;
}

interface Props {
  task?: ScheduledTask | null;
  onSubmit: (body: ScheduledTaskUpsertRequest) => Promise<void>;
  onCancel: () => void;
}

const DEFAULT_TARGET_CONFIG = {
  im_push: '{\n  "chat_id": ""\n}',
  session: '{\n  "session_name": ""\n}',
} satisfies Record<ScheduledTaskTargetType, string>;

function localTimezone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
}

function taskToForm(task?: ScheduledTask | null): FormState {
  if (!task) {
    return {
      name: '',
      description: '',
      targetType: 'session',
      targetConfigJson: DEFAULT_TARGET_CONFIG.session,
      platform: '',
      prompt: '',
      scheduleMode: 'cron',
      intervalSec: '86400',
      cronExpr: '0 9 * * *',
      timezone: localTimezone(),
      enabled: true,
    };
  }

  const scheduleMode: ScheduleMode = task.cron_expr ? 'cron' : 'interval';
  return {
    name: task.name,
    description: task.description ?? '',
    targetType: task.target_type,
    targetConfigJson: JSON.stringify(task.target_config ?? {}, null, 2),
    platform: task.platform ?? '',
    prompt: task.prompt,
    scheduleMode,
    intervalSec: String(task.interval_sec || 3600),
    cronExpr: task.cron_expr || '0 9 * * *',
    timezone: task.timezone || localTimezone(),
    enabled: task.enabled,
  };
}

function buildRequest(form: FormState): ScheduledTaskUpsertRequest {
  const targetConfig = form.targetConfigJson.trim()
    ? JSON.parse(form.targetConfigJson) as Record<string, unknown>
    : {};
  const intervalSec = Number(form.intervalSec);
  return {
    name: form.name.trim(),
    description: form.description.trim() || undefined,
    target_type: form.targetType,
    target_config: targetConfig,
    platform: form.targetType === 'im_push' ? form.platform.trim() || undefined : undefined,
    prompt: form.prompt.trim(),
    interval_sec: form.scheduleMode === 'interval' ? intervalSec : undefined,
    cron_expr: form.scheduleMode === 'cron' ? form.cronExpr.trim() : undefined,
    timezone: form.timezone.trim() || 'UTC',
    enabled: form.enabled,
  };
}

export function ScheduledTaskForm({ task, onSubmit, onCancel }: Props) {
  const { t } = useTranslation();
  const [form, setForm] = useState<FormState>(() => taskToForm(task));
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const isEdit = Boolean(task);

  const title = isEdit
    ? t('scheduledTasks.editTask', '编辑定时任务')
    : t('scheduledTasks.newTask', '新建定时任务');

  const scheduleHint = useMemo(() => {
    if (form.scheduleMode === 'cron') {
      return t('scheduledTasks.cronHint', '使用标准 5 字段 cron，例如每天 09:00: 0 9 * * *');
    }
    return t('scheduledTasks.intervalHint', '间隔以秒为单位，例如 3600 表示每小时执行一次。');
  }, [form.scheduleMode, t]);

  const set = <K extends keyof FormState>(key: K, value: FormState[K]) => {
    setForm((current) => ({ ...current, [key]: value }));
  };

  const setTargetType = (targetType: ScheduledTaskTargetType) => {
    setForm((current) => ({
      ...current,
      targetType,
      platform: targetType === 'im_push' ? current.platform : '',
      targetConfigJson: current.targetConfigJson.trim() ? current.targetConfigJson : DEFAULT_TARGET_CONFIG[targetType],
    }));
  };

  const handleSubmit = async () => {
    setError(null);
    if (!form.name.trim()) {
      setError(t('scheduledTasks.nameRequired', '任务名称不能为空'));
      return;
    }
    if (!form.prompt.trim()) {
      setError(t('scheduledTasks.promptRequired', '执行提示词不能为空'));
      return;
    }
    if (form.scheduleMode === 'interval') {
      const intervalSec = Number(form.intervalSec);
      if (!Number.isInteger(intervalSec) || intervalSec <= 0) {
        setError(t('scheduledTasks.intervalRequired', '间隔秒数必须是正整数'));
        return;
      }
    }
    if (form.scheduleMode === 'cron' && !form.cronExpr.trim()) {
      setError(t('scheduledTasks.cronRequired', 'cron 表达式不能为空'));
      return;
    }

    let body: ScheduledTaskUpsertRequest;
    try {
      body = buildRequest(form);
    } catch {
      setError(t('scheduledTasks.targetConfigInvalid', 'Target config 必须是合法 JSON'));
      return;
    }

    setSaving(true);
    try {
      await onSubmit(body);
    } catch (e) {
      setError(e instanceof Error ? e.message : t('scheduledTasks.saveFailed', '保存定时任务失败'));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex h-full flex-col bg-[var(--bg-card)]">
      <div className="flex items-center justify-between border-b border-[var(--border-color)] px-5 py-4">
        <div>
          <h2 className="text-base font-semibold text-[var(--text-primary)] font-display">{title}</h2>
          <p className="mt-0.5 text-xs text-[var(--text-secondary)]">
            {t('scheduledTasks.formSubtitle', '配置 target、调度规则和 Agent 执行提示词')}
          </p>
        </div>
        <button
          type="button"
          onClick={onCancel}
          className="rounded-lg p-2 text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]"
          aria-label={t('common.cancel', '取消')}
          title={t('common.cancel', '取消')}
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      <div className="flex-1 space-y-5 overflow-y-auto px-5 py-5">
        {error && (
          <div className="rounded-xl border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300">
            {error}
          </div>
        )}

        <section className="space-y-3">
          <SectionTitle>{t('scheduledTasks.basicInfo', '基础信息')}</SectionTitle>
          <Field label={t('scheduledTasks.name', '名称')} required>
            <input
              value={form.name}
              onChange={(e) => set('name', e.target.value)}
              className="w-full rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)]"
              placeholder={t('scheduledTasks.namePlaceholder', '每日质量巡检')}
              autoFocus
            />
          </Field>
          <Field label={t('scheduledTasks.description', '描述')}>
            <input
              value={form.description}
              onChange={(e) => set('description', e.target.value)}
              className="w-full rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)]"
              placeholder={t('scheduledTasks.descriptionPlaceholder', '用于后台列表识别任务用途')}
            />
          </Field>
          <label className="flex items-center gap-2 text-sm text-[var(--text-secondary)]">
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={(e) => set('enabled', e.target.checked)}
              className="rounded border-[var(--border-color)]"
            />
            {t('scheduledTasks.enabledOnSave', '保存后启用')}
          </label>
        </section>

        <section className="space-y-3 border-t border-[var(--border-color)] pt-5">
          <SectionTitle>{t('scheduledTasks.target', '执行目标')}</SectionTitle>
          <Field label={t('scheduledTasks.targetType', '目标类型')}>
            <div className="inline-flex rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] p-1">
              {(['session', 'im_push'] as ScheduledTaskTargetType[]).map((targetType) => (
                <button
                  key={targetType}
                  type="button"
                  onClick={() => setTargetType(targetType)}
                  className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                    form.targetType === targetType
                      ? 'bg-[var(--accent-600)] text-white'
                      : 'text-[var(--text-secondary)] hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]'
                  }`}
                >
                  {targetType === 'session' ? t('scheduledTasks.targetSession', 'Web Session') : t('scheduledTasks.targetImPush', 'IM 推送')}
                </button>
              ))}
            </div>
          </Field>
          {form.targetType === 'im_push' && (
            <Field label={t('scheduledTasks.platform', '平台')}>
              <select
                value={form.platform}
                onChange={(e) => set('platform', e.target.value)}
                className="w-full rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)]"
              >
                <option value="">{t('scheduledTasks.platformAuto', '不指定')}</option>
                <option value="feishu">feishu</option>
                <option value="dingtalk">dingtalk</option>
                <option value="wecom">wecom</option>
              </select>
            </Field>
          )}
          <Field label="target_config">
            <textarea
              value={form.targetConfigJson}
              onChange={(e) => set('targetConfigJson', e.target.value)}
              rows={5}
              spellCheck={false}
              className="w-full rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] px-3 py-2 font-mono text-xs leading-5 text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)]"
            />
          </Field>
        </section>

        <section className="space-y-3 border-t border-[var(--border-color)] pt-5">
          <SectionTitle>{t('scheduledTasks.schedule', '调度规则')}</SectionTitle>
          <Field label={t('scheduledTasks.scheduleMode', '模式')}>
            <div className="inline-flex rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] p-1">
              {(['cron', 'interval'] as ScheduleMode[]).map((mode) => (
                <button
                  key={mode}
                  type="button"
                  onClick={() => set('scheduleMode', mode)}
                  className={`rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                    form.scheduleMode === mode
                      ? 'bg-[var(--accent-600)] text-white'
                      : 'text-[var(--text-secondary)] hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]'
                  }`}
                >
                  {mode === 'cron' ? 'cron' : t('scheduledTasks.interval', '间隔')}
                </button>
              ))}
            </div>
          </Field>
          {form.scheduleMode === 'cron' ? (
            <Field label="cron_expr" hint={scheduleHint} required>
              <input
                value={form.cronExpr}
                onChange={(e) => set('cronExpr', e.target.value)}
                className="w-full rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] px-3 py-2 font-mono text-sm text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)]"
                placeholder="0 9 * * *"
              />
            </Field>
          ) : (
            <Field label="interval_sec" hint={scheduleHint} required>
              <input
                type="number"
                min={1}
                value={form.intervalSec}
                onChange={(e) => set('intervalSec', e.target.value)}
                className="w-full rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)]"
              />
            </Field>
          )}
          <Field label="timezone">
            <input
              value={form.timezone}
              onChange={(e) => set('timezone', e.target.value)}
              className="w-full rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] px-3 py-2 text-sm text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)]"
              placeholder="Asia/Shanghai"
            />
          </Field>
        </section>

        <section className="space-y-3 border-t border-[var(--border-color)] pt-5">
          <SectionTitle>{t('scheduledTasks.prompt', '执行提示词')}</SectionTitle>
          <textarea
            value={form.prompt}
            onChange={(e) => set('prompt', e.target.value)}
            rows={7}
            className="w-full rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] px-3 py-2 text-sm leading-6 text-[var(--text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--accent-subtle)]"
            placeholder={t('scheduledTasks.promptPlaceholder', '检查当前仓库测试/质量风险并生成报告')}
          />
        </section>
      </div>

      <div className="flex justify-end gap-2 border-t border-[var(--border-color)] px-5 py-4">
        <button
          type="button"
          onClick={onCancel}
          className="inline-flex items-center gap-1.5 rounded-lg border border-[var(--border-color)] px-3 py-2 text-sm text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]"
        >
          <X className="h-4 w-4" />
          {t('common.cancel', '取消')}
        </button>
        <button
          type="button"
          onClick={handleSubmit}
          disabled={saving}
          className="inline-flex items-center gap-1.5 rounded-lg bg-[var(--accent-600)] px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-[var(--accent-700)] disabled:opacity-50"
        >
          <Check className="h-4 w-4" />
          {saving ? t('common.saving', '保存中...') : t('common.save', '保存')}
        </button>
      </div>
    </div>
  );
}

function SectionTitle({ children }: { children: React.ReactNode }) {
  return <h3 className="text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]">{children}</h3>;
}

function Field({ label, children, hint, required }: {
  label: string;
  children: React.ReactNode;
  hint?: string;
  required?: boolean;
}) {
  return (
    <label className="block">
      <span className="mb-1 flex items-center gap-1 text-xs font-medium text-[var(--text-secondary)]">
        {label}
        {required && <span className="text-red-500">*</span>}
      </span>
      {children}
      {hint && <span className="mt-1 block text-xs leading-5 text-[var(--text-secondary)]">{hint}</span>}
    </label>
  );
}
