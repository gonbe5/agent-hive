import { useTranslation } from 'react-i18next';
import { Circle, Loader2, CheckCircle2, XCircle, Layers } from 'lucide-react';
import { useTaskProgressStore } from '../../store/taskProgress';
import { getToolDisplayName } from '../../utils/toolName';

// 状态图标映射
function StatusIcon({ status }: { status: string }) {
  switch (status) {
    case 'pending':
      return <Circle className="w-4 h-4 text-[var(--text-secondary)]" />;
    case 'running':
      return <Loader2 className="w-4 h-4 text-[var(--accent-500)] animate-spin" />;
    case 'completed':
      return <CheckCircle2 className="w-4 h-4 text-green-500" />;
    case 'failed':
      return <XCircle className="w-4 h-4 text-red-500" />;
    default:
      return <Circle className="w-4 h-4 text-[var(--text-secondary)]" />;
  }
}

// 进度条组件
function ProgressBar({ completed, total }: { completed: number; total: number }) {
  const pct = total > 0 ? Math.round((completed / total) * 100) : 0;
  return (
    <div className="w-full h-2 rounded-full bg-[var(--bg-secondary)] overflow-hidden mt-2">
      <div
        className="h-full rounded-full task-progress-bar transition-all duration-300"
        style={{ width: `${pct}%` }}
      />
    </div>
  );
}

// 并行任务组视图
function TaskGroupView() {
  const { t } = useTranslation();
  const groups = useTaskProgressStore((s) => s.activeGroups);

  if (groups.size === 0) return null;

  return (
    <>
      {Array.from(groups.values()).map((group) => (
          <div key={group.groupId} className={`task-progress-panel mb-3 ${group.status !== 'running' ? 'opacity-60' : ''}`}>
            <div className="flex items-center gap-2 mb-2">
              <Layers className="w-4 h-4 text-[var(--accent-500)]" />
              <span className="font-semibold text-[var(--text-primary)]">
                {t('task.parallelTitle')} ({group.completed}/{group.total})
              </span>
              {group.status !== 'running' && (
                <span className={`text-xs ml-auto ${group.status === 'completed' ? 'text-green-500' : 'text-red-400'}`}>
                  {t(`task.${group.status}`)}
                </span>
              )}
            </div>

            <div className="space-y-1.5">
              {group.tasks.map((task) => (
                <div key={task.id}>
                  <div className="flex items-center gap-2 text-sm">
                    <StatusIcon status={task.status} />
                    <span className="text-[var(--text-primary)] truncate">{task.instruction}</span>
                    <span className="text-xs text-[var(--text-secondary)]">
                      {t(`task.${task.status}`)}
                    </span>
                  </div>
                  {task.toolProgress && (
                    <div className="ml-7 text-xs text-[var(--text-secondary)] flex items-center gap-1 mt-0.5">
                      <span className="font-mono">{getToolDisplayName(task.toolProgress.toolName, t)}</span>
                      <span>-</span>
                      <span>{task.toolProgress.status}</span>
                      {task.toolProgress.turn > 0 && (
                        <span className="ml-1">
                          {t('task.turn', { current: task.toolProgress.turn, max: task.toolProgress.maxTurns })}
                        </span>
                      )}
                    </div>
                  )}
                  {task.error && (
                    <div className="ml-7 text-xs text-red-400 mt-0.5">{task.error}</div>
                  )}
                </div>
              ))}
            </div>

            <ProgressBar completed={group.completed} total={group.total} />
          </div>
        ))}
    </>
  );
}

export function TaskProgressPanel() {
  const groups = useTaskProgressStore((s) => s.activeGroups);

  if (groups.size === 0) return null;

  return (
    <div className="px-4 pb-1">
      <div className="max-w-3xl mx-auto">
        <TaskGroupView />
      </div>
    </div>
  );
}
