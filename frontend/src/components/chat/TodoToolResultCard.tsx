import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { CheckCircle2, Circle, ClipboardList, Clock3, MinusCircle } from 'lucide-react';
import type { PlanStatus, TodoStatus } from '../../store/todos';

interface InlineTodo {
  id: string;
  content: string;
  status: TodoStatus;
  order: number;
}

interface InlineTodoSnapshot {
  plan_status: PlanStatus;
  plan_version?: number;
  todos: InlineTodo[];
}

interface TodoToolResultCardProps {
  result?: string;
}

const TODO_STATUSES = new Set<TodoStatus>(['pending', 'in_progress', 'completed', 'cancelled']);
const PLAN_STATUSES = new Set<PlanStatus>([
  'none',
  'planning',
  'awaiting_approval',
  'executing',
  'paused',
  'completed',
  'failed',
]);

export function isTodoWriteTool(name: string): boolean {
  const normalized = name.toLowerCase().replace(/[-\s]/g, '_');
  return normalized === 'todo_write' || normalized === 'todowrite';
}

export function parseTodoToolSnapshot(result?: string): InlineTodoSnapshot | null {
  if (!result) return null;

  let parsed: unknown;
  try {
    parsed = JSON.parse(result);
  } catch {
    return null;
  }

  if (!parsed || typeof parsed !== 'object') return null;
  const value = parsed as Record<string, unknown>;
  if (!Array.isArray(value.todos)) return null;
  if (typeof value.plan_status !== 'string' || !PLAN_STATUSES.has(value.plan_status as PlanStatus)) {
    return null;
  }

  const todos = value.todos
    .map((raw, index): InlineTodo | null => {
      if (!raw || typeof raw !== 'object') return null;
      const item = raw as Record<string, unknown>;
      if (typeof item.id !== 'string' || typeof item.content !== 'string') return null;
      const status = typeof item.status === 'string' && TODO_STATUSES.has(item.status as TodoStatus)
        ? item.status as TodoStatus
        : 'pending';
      const order = typeof item.order === 'number' ? item.order : index;
      return { id: item.id, content: item.content, status, order };
    })
    .filter((todo): todo is InlineTodo => todo !== null)
    .sort((a, b) => a.order - b.order);

  return {
    plan_status: value.plan_status as PlanStatus,
    plan_version: typeof value.plan_version === 'number' ? value.plan_version : undefined,
    todos,
  };
}

export function TodoToolResultCard({ result }: TodoToolResultCardProps) {
  const { t } = useTranslation();
  const snapshot = useMemo(() => parseTodoToolSnapshot(result), [result]);

  if (!snapshot) return null;

  return (
    <section
      data-testid="inline-todos-list"
      className="not-prose mb-3 max-w-3xl"
      aria-label={t('todos.ariaLabel')}
    >
      <div className="mb-1.5 flex items-center gap-2 px-1">
        <ClipboardList className="h-4 w-4 shrink-0 text-[var(--accent-600)] dark:text-[var(--accent-300)]" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <div className="text-sm font-semibold leading-5 text-[var(--text-primary)]">
            {t('todos.inlineUpdated')}
          </div>
          <div className="text-[11px] leading-4 text-[var(--text-secondary)]">
            {t(`todos.planStatus.${snapshot.plan_status}`)}
            {snapshot.plan_version !== undefined ? ` · v${snapshot.plan_version}` : ''}
          </div>
        </div>
        <span className="shrink-0 text-[11px] font-medium text-[var(--text-secondary)]">
          {t('todos.taskCount', { count: snapshot.todos.length })}
        </span>
      </div>

      {snapshot.todos.length > 0 ? (
        <ol className="space-y-0.5">
          {snapshot.todos.map((todo) => (
            <InlineTodoItem key={todo.id} todo={todo} />
          ))}
        </ol>
      ) : (
        <p className="px-1 py-1.5 text-xs text-[var(--text-secondary)]">
          {t('todos.emptyActivePlan')}
        </p>
      )}
    </section>
  );
}

function InlineTodoItem({ todo }: { todo: InlineTodo }) {
  const { t } = useTranslation();
  const statusLabel = t(`todos.status.${todo.status}`);
  const isMuted = todo.status === 'completed' || todo.status === 'cancelled';

  return (
    <li className="grid grid-cols-[18px_minmax(0,1fr)_auto] items-start gap-2.5 rounded-md px-1 py-1.5 text-sm hover:bg-[var(--bg-secondary)]">
      <span className={`mt-0.5 ${statusTextColor(todo.status)}`}>
        <TodoStatusIcon status={todo.status} label={statusLabel} />
      </span>
      <span className={`${isMuted ? 'text-[var(--text-secondary)]' : 'text-[var(--text-primary)]'} break-words leading-5`}>
        {todo.content}
      </span>
      <span className="whitespace-nowrap pt-0.5 text-[11px] font-medium text-[var(--text-secondary)]">
        {statusLabel}
      </span>
    </li>
  );
}

function TodoStatusIcon({ status, label }: { status: TodoStatus; label: string }) {
  switch (status) {
    case 'in_progress':
      return <Clock3 className="h-4 w-4" aria-label={label} />;
    case 'completed':
      return <CheckCircle2 className="h-4 w-4" aria-label={label} />;
    case 'cancelled':
      return <MinusCircle className="h-4 w-4" aria-label={label} />;
    case 'pending':
    default:
      return <Circle className="h-4 w-4" aria-label={label} />;
  }
}

function statusTextColor(status: TodoStatus): string {
  switch (status) {
    case 'in_progress':
      return 'text-[var(--accent-600)] dark:text-[var(--accent-300)]';
    case 'completed':
      return 'text-[var(--success)]';
    case 'cancelled':
      return 'text-[var(--text-secondary)]';
    case 'pending':
    default:
      return 'text-[var(--text-secondary)]';
  }
}
