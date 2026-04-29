import { useAppStore } from '../store/app';

/** 生成 RFC3339 格式 timestamp（无毫秒），与后端 time.RFC3339 一致 */
export function rfc3339Now(): string {
  return new Date().toISOString().replace(/\.\d{3}Z$/, 'Z');
}

/**
 * 判断字符串是否是合法时间戳（真正可被 Date 解析）。
 * 前端内部标记如 "stream-xxx"、"temp-xxx" 会被 new Date 当成 Invalid Date，统一返回 false。
 */
export function isValidTimestamp(date: string | Date | undefined | null): boolean {
  if (date == null || date === '') return false;
  if (typeof date === 'string') {
    if (date.startsWith('stream-') || date.startsWith('temp-')) return false;
  }
  const d = typeof date === 'string' ? new Date(date) : date;
  return !isNaN(d.getTime());
}

/**
 * 格式化日期时间，根据当前语言设置。无效值返回 ''。
 */
export function formatDateTime(date: string | Date): string {
  const language = useAppStore.getState().language;
  const locale = language === 'zh' ? 'zh-CN' : 'en-US';

  try {
    if (!isValidTimestamp(date)) return '';
    const dateObj = typeof date === 'string' ? new Date(date) : date;
    return dateObj.toLocaleString(locale, { hour12: false });
  } catch {
    return '';
  }
}

/**
 * 仅返回时间部分（HH:MM:SS），无效值返回 ''。
 * 替代易碎的 formatDateTime(x).split(' ')[1] 模式（当 Date 非法时会吐出 "Date"）。
 */
export function formatTimeOnly(date: string | Date): string {
  try {
    if (!isValidTimestamp(date)) return '';
    const language = useAppStore.getState().language;
    const locale = language === 'zh' ? 'zh-CN' : 'en-US';
    const dateObj = typeof date === 'string' ? new Date(date) : date;
    return dateObj.toLocaleTimeString(locale, { hour12: false });
  } catch {
    return '';
  }
}
