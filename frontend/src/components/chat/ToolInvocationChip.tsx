import { useTranslation } from 'react-i18next';
import { Settings, Loader2 } from 'lucide-react';
import { getToolDisplayName } from '../../utils/toolName';

interface ToolInvocationChipProps {
  name: string;
  status?: 'running' | 'success' | 'error';
}

/**
 * 工具调用 chip — 完全无状态（design.md D2）。
 * live running/success 状态由上层（ToolCallRow）解析后通过 `status` 注入。
 */
export function ToolInvocationChip({ name, status }: ToolInvocationChipProps) {
  const { t } = useTranslation();

  const resolvedStatus: 'running' | 'success' | 'error' = status ?? 'success';

  const displayName = getToolDisplayName(name, t);
  const label = `${t('tools.invoked')}: ${displayName}`;

  const isRunning = resolvedStatus === 'running';
  const isError = resolvedStatus === 'error';

  const iconColor = isError
    ? 'text-[var(--danger)]'
    : 'text-[var(--accent-600)] dark:text-[var(--accent-300)]';

  const textColor = isError
    ? 'text-[var(--danger)]'
    : 'text-[var(--accent-700)] dark:text-[var(--accent-300)]';

  const borderColor = isError
    ? 'border-[var(--danger)]/30'
    : 'border-[var(--accent-200)] dark:border-[var(--accent-700)]/40';

  const Icon = isRunning ? Loader2 : Settings;

  return (
    <span
      role="status"
      aria-label={label}
      className={`inline-flex items-center gap-1.5 px-3 py-1.5 rounded-full border bg-[var(--accent-subtle)] text-xs font-medium ${borderColor} ${textColor}`}
    >
      <Icon
        className={`w-3.5 h-3.5 ${iconColor} ${isRunning ? 'animate-spin' : ''}`}
        aria-hidden="true"
      />
      <span>{displayName}</span>
    </span>
  );
}

export default ToolInvocationChip;
