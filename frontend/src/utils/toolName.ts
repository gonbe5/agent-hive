import type { TFunction } from 'i18next';

/**
 * 获取工具的中文显示名称。
 * 优先级：完整名称翻译 > MCP 工具拆分翻译 > humanize fallback
 */
export function getToolDisplayName(name: string, t: TFunction): string {
  if (!name) return '';

  // 1. 先尝试完整名称翻译
  const fullKey = `tools.${name.toLowerCase()}`;
  const full = t(fullKey, '');
  if (full) return full;

  // 2. MCP 工具：server__tool_name → [server] 中文名
  if (name.includes('__')) {
    const idx = name.indexOf('__');
    const serverName = name.slice(0, idx);
    const toolPart = name.slice(idx + 2);
    const toolTranslation = t(`tools.${toolPart.toLowerCase()}`, '');
    if (toolTranslation) return `[${serverName}] ${toolTranslation}`;
    return `[${serverName}] ${humanize(toolPart)}`;
  }

  // 3. 无翻译：humanize 处理
  return humanize(name);
}

function humanize(s: string): string {
  return s
    .replace(/_/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase());
}
