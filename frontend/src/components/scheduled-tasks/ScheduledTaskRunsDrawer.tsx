import { Link } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { CheckCircle2, ExternalLink, Loader2, RefreshCw, X, XCircle } from 'lucide-react';
import type { ScheduledTask, ScheduledTaskRun, ScheduledTaskRunStatus } from '../../types/api';
import { formatDateTime } from '../../utils/date';

interface Props {
  task: ScheduledTask | null;
  runs: ScheduledTaskRun[];
  loading: boolean;
  onClose: () => void;
  onRefresh: () => void;
}

function statusClass(status: ScheduledTaskRunStatus): string {
  switch (status) {
    case 'succeeded':
      return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300';
    case 'running':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300';
    case 'skipped':
      return 'bg-[var(--bg-secondary)] text-[var(--text-secondary)]';
    case 'timeout':
      return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300';
    case 'failed':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300';
  }
}

function durationMs(run: ScheduledTaskRun): number | null {
  if (!run.finished_at) return null;
  const started = new Date(run.started_at).getTime();
  const finished = new Date(run.finished_at).getTime();
  if (!Number.isFinite(started) || !Number.isFinite(finished) || finished < started) return null;
  return finished - started;
}

function formatDuration(ms: number | null): string {
  if (ms == null) return '';
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  return `${Math.round(ms / 60_000)}m ${Math.round((ms % 60_000) / 1000)}s`;
}

export function ScheduledTaskRunsDrawer({ task, runs, loading, onClose, onRefresh }: Props) {
  const { t } = useTranslation();
  if (!task) return null;

  return (
    <div
      role="dialog"
      aria-modal="false"
      aria-labelledby="scheduled-task-runs-title"
      className="fixed inset-y-0 right-0 z-40 flex w-[440px] max-w-[calc(100vw-16rem)] flex-col border-l border-[var(--border-color)] bg-[var(--bg-card)] shadow-2xl"
    >
      <div className="flex items-start justify-between border-b border-[var(--border-color)] px-5 py-4">
        <div className="min-w-0">
          <h2 id="scheduled-task-runs-title" className="truncate text-base font-semibold text-[var(--text-primary)] font-display">
            {task.name}
          </h2>
          <p className="mt-1 text-xs text-[var(--text-secondary)]">
            {t('scheduledTasks.runHistory', '运行记录')}
          </p>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <button
            type="button"
            onClick={onRefresh}
            className="rounded-lg p-2 text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]"
            aria-label={t('common.refresh', '刷新')}
            title={t('common.refresh', '刷新')}
          >
            <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
          </button>
          <button
            type="button"
            onClick={onClose}
            className="rounded-lg p-2 text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]"
            aria-label={t('common.close', '关闭')}
            title={t('common.close', '关闭')}
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      </div>

      <div className="flex-1 overflow-y-auto px-5 py-4">
        {loading ? (
          <div className="flex items-center justify-center py-16 text-sm text-[var(--text-secondary)]">
            <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            {t('common.loading', '加载中...')}
          </div>
        ) : runs.length === 0 ? (
          <div className="rounded-2xl border border-dashed border-[var(--border-color)] px-5 py-10 text-center">
            <p className="text-sm text-[var(--text-secondary)]">{t('scheduledTasks.noRuns', '暂无运行记录')}</p>
          </div>
        ) : (
          <div className="space-y-3">
            {runs.map((run) => (
              <article
                key={`${run.scheduled_at}-${run.id}`}
                className="rounded-2xl border border-[var(--border-color)] bg-[var(--bg-primary)] p-4"
              >
                <div className="mb-3 flex items-center justify-between gap-3">
                  <span className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium ${statusClass(run.status)}`}>
                    {run.status === 'succeeded' ? <CheckCircle2 className="h-3.5 w-3.5" /> : run.status === 'failed' ? <XCircle className="h-3.5 w-3.5" /> : null}
                    {run.status}
                  </span>
                  <span className="text-xs text-[var(--text-secondary)]">
                    {formatDuration(durationMs(run))}
                  </span>
                </div>
                <dl className="grid grid-cols-[88px_1fr] gap-x-3 gap-y-2 text-xs">
                  <dt className="text-[var(--text-secondary)]">{t('scheduledTasks.scheduledAt', '计划时间')}</dt>
                  <dd className="text-[var(--text-primary)]">{formatDateTime(run.scheduled_at)}</dd>
                  <dt className="text-[var(--text-secondary)]">{t('scheduledTasks.startedAt', '开始时间')}</dt>
                  <dd className="text-[var(--text-primary)]">{formatDateTime(run.started_at)}</dd>
                  {run.finished_at && (
                    <>
                      <dt className="text-[var(--text-secondary)]">{t('scheduledTasks.finishedAt', '结束时间')}</dt>
                      <dd className="text-[var(--text-primary)]">{formatDateTime(run.finished_at)}</dd>
                    </>
                  )}
                  <dt className="text-[var(--text-secondary)]">{t('scheduledTasks.attempts', '尝试')}</dt>
                  <dd className="text-[var(--text-primary)]">{run.attempt_count}</dd>
                  {run.session_id && (
                    <>
                      <dt className="text-[var(--text-secondary)]">session</dt>
                      <dd>
                        <Link
                          to={`/sessions/${run.session_id}`}
                          className="inline-flex items-center gap-1 text-[var(--accent-600)] hover:text-[var(--accent-700)] dark:text-[var(--accent-300)]"
                        >
                          {run.session_id}
                          <ExternalLink className="h-3 w-3" />
                        </Link>
                      </dd>
                    </>
                  )}
                </dl>
                {(run.error || run.output) && (
                  <div className="mt-3 rounded-lg border border-[var(--border-color)] bg-[var(--bg-card)] p-3">
                    <div className="mb-1 text-xs font-medium text-[var(--text-secondary)]">
                      {run.error ? t('scheduledTasks.errorOutput', '错误') : t('scheduledTasks.output', '输出')}
                    </div>
                    <pre className="max-h-44 overflow-auto whitespace-pre-wrap break-words font-mono text-xs leading-5 text-[var(--text-primary)]">
                      {run.error || run.output}
                    </pre>
                  </div>
                )}
              </article>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
