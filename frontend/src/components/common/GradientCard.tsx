import type { ReactNode } from 'react';

interface Props {
  label: string;
  value: string;
  icon?: ReactNode;
  accent?: 'emerald' | 'red' | 'amber' | 'blue';
  dot?: boolean;
}

/** 统计卡片（Dashboard 用） */
export function GradientCard({ label, value, icon, accent = 'amber', dot }: Props) {
  // 状态点颜色映射
  const dotColorMap: Record<string, string> = {
    emerald: 'bg-emerald-500',
    red: 'bg-red-500',
    /* warning semantic */
    amber: 'bg-[var(--warning)]',
    blue: 'bg-blue-500',
  };

  return (
    <div className="bg-[var(--bg-card)] border border-[var(--border-color)] rounded-2xl shadow-sm transition-shadow">
      <div className="p-4">
        <div className="text-xs text-[var(--text-secondary)] mb-2 flex items-center gap-2">
          {icon}
          {label}
        </div>
        <div className="text-xl font-semibold text-[var(--text-primary)] flex items-center gap-2">
          {dot && <span className={`w-2 h-2 rounded-full ${dotColorMap[accent]} animate-pulse`} />}
          {value}
        </div>
      </div>
    </div>
  );
}
