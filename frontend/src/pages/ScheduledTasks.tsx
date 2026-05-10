import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { AlertTriangle, CalendarClock, Plus, RefreshCw } from 'lucide-react';
import { useNodeClient } from '../hooks/useNodeClient';
import { useScheduledTasksStore } from '../store/scheduledTasks';
import { ScheduledTaskForm } from '../components/scheduled-tasks/ScheduledTaskForm';
import { ScheduledTaskRunsDrawer } from '../components/scheduled-tasks/ScheduledTaskRunsDrawer';
import { ScheduledTaskTable } from '../components/scheduled-tasks/ScheduledTaskTable';
import type { ScheduledTask, ScheduledTaskUpsertRequest } from '../types/api';

type FormMode =
  | { type: 'closed' }
  | { type: 'create' }
  | { type: 'edit'; task: ScheduledTask };

export function ScheduledTasks() {
  const { t } = useTranslation();
  const client = useNodeClient();
  const {
    tasks,
    runsByTaskId,
    loading,
    runsLoadingByTaskId,
    error,
    partialWarning,
    runningNowTaskId,
    loadTasks,
    createTask,
    updateTask,
    deleteTask,
    toggleTask,
    runNow,
    loadRuns,
  } = useScheduledTasksStore();

  const [formMode, setFormMode] = useState<FormMode>({ type: 'closed' });
  const [expandedTaskId, setExpandedTaskId] = useState<string | null>(null);
  const [runsDrawerTask, setRunsDrawerTask] = useState<ScheduledTask | null>(null);
  const [runNowAnnouncement, setRunNowAnnouncement] = useState('');

  useEffect(() => {
    loadTasks(client);
  }, [client, loadTasks]);

  const openRunsDrawer = (task: ScheduledTask) => {
    setRunsDrawerTask(task);
    loadRuns(client, task.id, 20);
  };

  const toggleExpanded = (task: ScheduledTask) => {
    const nextExpanded = expandedTaskId === task.id ? null : task.id;
    setExpandedTaskId(nextExpanded);
    if (nextExpanded) {
      loadRuns(client, task.id, 5);
    }
  };

  const handleRunNow = async (task: ScheduledTask) => {
    setRunNowAnnouncement(t('scheduledTasks.runNowStarted', '正在触发 "{{name}}" 立即运行', { name: task.name }));
    try {
      await runNow(client, task.id);
      setRunNowAnnouncement(t('scheduledTasks.runNowQueued', '"{{name}}" 已触发立即运行', { name: task.name }));
      if (runsDrawerTask?.id === task.id) {
        loadRuns(client, task.id, 20);
      }
    } catch {
      setRunNowAnnouncement(t('scheduledTasks.runNowFailed', '"{{name}}" 立即运行失败', { name: task.name }));
    }
  };

  const handleSubmitForm = async (body: ScheduledTaskUpsertRequest) => {
    if (formMode.type === 'edit') {
      await updateTask(client, formMode.task.id, body);
    } else {
      await createTask(client, body);
    }
    setFormMode({ type: 'closed' });
  };

  const handleDelete = async (task: ScheduledTask) => {
    await deleteTask(client, task.id);
    if (expandedTaskId === task.id) setExpandedTaskId(null);
    if (runsDrawerTask?.id === task.id) setRunsDrawerTask(null);
  };

  const selectedDrawerRuns = runsDrawerTask ? runsByTaskId[runsDrawerTask.id] ?? [] : [];
  const selectedDrawerLoading = runsDrawerTask ? runsLoadingByTaskId[runsDrawerTask.id] ?? false : false;

  return (
    <div className="min-h-full">
      <div className="md:hidden flex min-h-full items-center justify-center p-6">
        <div className="rounded-2xl border border-[var(--border-color)] bg-[var(--bg-card)] p-6 text-center shadow-sm">
          <CalendarClock className="mx-auto mb-3 h-8 w-8 text-[var(--accent-600)]" />
          <p className="text-sm text-[var(--text-primary)]">
            {t('scheduledTasks.desktopOnly', '请在桌面浏览器使用定时任务管理')}
          </p>
        </div>
      </div>

      <div className="hidden md:block">
        <div className={`mx-auto max-w-6xl p-6 ${runsDrawerTask ? 'mr-[456px]' : ''}`}>
          <div className="mb-6 flex items-start justify-between gap-4">
            <div>
              <h1 className="text-xl font-semibold text-[var(--text-primary)] font-display">
                {t('scheduledTasks.title', '定时任务')}
              </h1>
              <p className="mt-1 text-sm text-[var(--text-secondary)]">
                {t('scheduledTasks.subtitle', '让 Agent 按 cron 或间隔自动执行巡检、播报和后台任务。')}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <button
                type="button"
                onClick={() => loadTasks(client)}
                disabled={loading}
                className="inline-flex items-center gap-1.5 rounded-lg border border-[var(--border-color)] px-3 py-2 text-sm text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)] disabled:opacity-50"
              >
                <RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
                {t('common.refresh', '刷新')}
              </button>
              <button
                type="button"
                onClick={() => setFormMode({ type: 'create' })}
                className="inline-flex items-center gap-1.5 rounded-lg bg-[var(--accent-600)] px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-[var(--accent-700)]"
              >
                <Plus className="h-4 w-4" />
                {t('scheduledTasks.newTask', '新建定时任务')}
              </button>
            </div>
          </div>

          {partialWarning && (
            <div className="mb-4 flex items-center gap-2 rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-900/60 dark:bg-amber-900/20 dark:text-amber-200">
              <AlertTriangle className="h-4 w-4 shrink-0" />
              <span>{t('scheduledTasks.partialWarning', '部分运行记录加载失败，任务列表仍可使用。')} {partialWarning}</span>
            </div>
          )}

          {loading ? (
            <ScheduledTasksSkeleton />
          ) : error ? (
            <ErrorState message={error} onRetry={() => loadTasks(client)} />
          ) : tasks.length === 0 ? (
            <EmptyState onCreate={() => setFormMode({ type: 'create' })} />
          ) : (
            <ScheduledTaskTable
              tasks={tasks}
              expandedTaskId={expandedTaskId}
              runsByTaskId={runsByTaskId}
              runsLoadingByTaskId={runsLoadingByTaskId}
              runningNowTaskId={runningNowTaskId}
              onToggleExpanded={toggleExpanded}
              onOpenRuns={openRunsDrawer}
              onEdit={(task) => setFormMode({ type: 'edit', task })}
              onRunNow={handleRunNow}
              onDelete={handleDelete}
              onToggleEnabled={(task, enabled) => toggleTask(client, task.id, enabled)}
            />
          )}
        </div>
      </div>

      <div className="sr-only" aria-live="polite" aria-atomic="true">
        {runNowAnnouncement}
      </div>

      {formMode.type !== 'closed' && (
        <div className="fixed inset-0 z-50 hidden items-center justify-center bg-black/40 p-6 backdrop-blur-sm md:flex">
          <div className="h-[min(820px,calc(100vh-48px))] w-[520px] overflow-hidden rounded-2xl border border-[var(--border-color)] bg-[var(--bg-card)] shadow-2xl">
            <ScheduledTaskForm
              task={formMode.type === 'edit' ? formMode.task : null}
              onSubmit={handleSubmitForm}
              onCancel={() => setFormMode({ type: 'closed' })}
            />
          </div>
        </div>
      )}

      <ScheduledTaskRunsDrawer
        task={runsDrawerTask}
        runs={selectedDrawerRuns}
        loading={selectedDrawerLoading}
        onClose={() => setRunsDrawerTask(null)}
        onRefresh={() => {
          if (runsDrawerTask) loadRuns(client, runsDrawerTask.id, 20);
        }}
      />
    </div>
  );
}

function ScheduledTasksSkeleton() {
  return (
    <div className="rounded-2xl border border-[var(--border-color)] bg-[var(--bg-card)] p-4 shadow-sm">
      <div className="mb-4 h-8 w-44 animate-pulse rounded-lg bg-[var(--bg-secondary)]" />
      <div className="space-y-3">
        {Array.from({ length: 6 }).map((_, index) => (
          <div key={index} className="grid grid-cols-[2fr_1fr_1fr_1fr] gap-4 rounded-xl border border-[var(--border-color)] p-4">
            <div className="space-y-2">
              <div className="h-4 w-40 animate-pulse rounded bg-[var(--bg-secondary)]" />
              <div className="h-3 w-64 animate-pulse rounded bg-[var(--bg-secondary)]" />
            </div>
            <div className="h-4 animate-pulse rounded bg-[var(--bg-secondary)]" />
            <div className="h-4 animate-pulse rounded bg-[var(--bg-secondary)]" />
            <div className="h-4 animate-pulse rounded bg-[var(--bg-secondary)]" />
          </div>
        ))}
      </div>
    </div>
  );
}

function EmptyState({ onCreate }: { onCreate: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="rounded-2xl border border-dashed border-[var(--border-color)] bg-[var(--bg-card)] px-6 py-16 text-center shadow-sm">
      <CalendarClock className="mx-auto mb-4 h-10 w-10 text-[var(--accent-600)]" />
      <h2 className="text-base font-semibold text-[var(--text-primary)] font-display">
        {t('scheduledTasks.emptyTitle', '暂无定时任务')}
      </h2>
      <p className="mx-auto mt-2 max-w-md text-sm text-[var(--text-secondary)]">
        {t('scheduledTasks.emptyDesc', '创建一个任务，让 Agent 定时新建 Session 或向 IM 频道推送消息。')}
      </p>
      <button
        type="button"
        onClick={onCreate}
        className="mt-5 inline-flex items-center gap-1.5 rounded-lg bg-[var(--accent-600)] px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-[var(--accent-700)]"
      >
        <Plus className="h-4 w-4" />
        {t('scheduledTasks.newTask', '新建定时任务')}
      </button>
    </div>
  );
}

function ErrorState({ message, onRetry }: { message: string; onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="rounded-2xl border border-red-200 bg-red-50 px-6 py-12 text-center shadow-sm dark:border-red-900/60 dark:bg-red-900/20">
      <AlertTriangle className="mx-auto mb-3 h-8 w-8 text-red-600 dark:text-red-300" />
      <h2 className="text-base font-semibold text-red-700 dark:text-red-200">
        {t('scheduledTasks.loadFailed', '加载定时任务失败')}
      </h2>
      <p className="mx-auto mt-2 max-w-lg text-sm text-red-700/80 dark:text-red-200/80">{message}</p>
      <button
        type="button"
        onClick={onRetry}
        className="mt-5 inline-flex items-center gap-1.5 rounded-lg bg-red-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700"
      >
        <RefreshCw className="h-4 w-4" />
        {t('scheduledTasks.retry', '重试')}
      </button>
    </div>
  );
}
