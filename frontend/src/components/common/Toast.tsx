import { X, CheckCircle, AlertCircle, AlertTriangle, Info } from 'lucide-react';
import { useToastStore } from '../../store/toast';

export function ToastContainer() {
  const toasts = useToastStore((s) => s.toasts);
  const removeToast = useToastStore((s) => s.removeToast);

  if (toasts.length === 0) return null;

  const typeConfig = {
    success: { icon: CheckCircle, cls: 'text-emerald-600 dark:text-emerald-400' },
    error:   { icon: AlertCircle, cls: 'text-red-600 dark:text-red-400' },
    warning: { icon: AlertTriangle, cls: 'text-[var(--warning)] dark:text-[var(--warning)]' },
    info:    { icon: Info, cls: 'text-blue-600 dark:text-blue-400' },
  };

  return (
    <div className="fixed top-4 right-4 z-[100] space-y-2 pointer-events-none">
      {toasts.map((toast) => {
        const { icon: Icon, cls } = typeConfig[toast.type];
        return (
          <div
            key={toast.id}
            className="apple-card px-4 py-3 text-sm pointer-events-auto animate-slide-in-right flex items-center gap-3 min-w-[280px] max-w-md"
          >
            <Icon className={`w-4 h-4 shrink-0 ${cls}`} />
            <span className="flex-1 text-[var(--text-primary)]">{toast.message}</span>
            <button
              onClick={() => removeToast(toast.id)}
              className="text-[var(--text-secondary)] hover:text-[var(--text-primary)] transition-colors"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        );
      })}
    </div>
  );
}
