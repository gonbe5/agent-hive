export type JournalEventType = 'tool_call' | 'file_change' | 'decision';

export interface JournalEvent {
  type: JournalEventType;
  timestamp: string;
  // tool_call 字段
  tool_name?: string;
  arguments?: string;
  result?: string;
  is_error?: boolean;
  duration_ms?: number;
  // file_change 字段
  file_path?: string;
  action?: 'create' | 'edit' | 'delete';
  summary?: string;
  // decision 字段
  decision?: string;
  reason?: string;
}

export interface JournalResponse {
  session_id: string;
  events: JournalEvent[];
}

export interface JournalStats {
  tool_call_count: number;
  file_change_count: number;
  decision_count: number;
  started_at: string;
  ended_at?: string;
  has_error: boolean;
}

export interface JournalStatsResponse {
  stats: Record<string, JournalStats | null>;
}

export type CharacterState = 'idle' | 'thinking' | 'reading' | 'coding' | 'running' | 'success' | 'error';

export function getCharacterState(event: JournalEvent): CharacterState {
  if (event.type === 'decision') return 'thinking';
  if (event.is_error) return 'error';
  if (event.type === 'file_change') return 'coding';
  if (event.type !== 'tool_call') return 'idle';

  const tool = event.tool_name || '';

  const readTools = [
    'read_file', 'glob', 'grep', 'ls',
    'web_search', 'web_fetch',
    'lsp_hover', 'lsp_definition', 'lsp_references',
    'lsp_symbols', 'lsp_diagnostics', 'lsp_completion',
    'memory',
  ];
  if (readTools.includes(tool)) return 'reading';

  const writeTools = [
    'write_file', 'edit', 'multi_edit', 'apply_patch',
    'create_tool', 'remove_tool',
    'lsp_rename', 'lsp_format', 'lsp_actions',
  ];
  if (writeTools.includes(tool)) return 'coding';

  const runTools = [
    'bash',
    'spawn_agent', 'parallel_dispatch',
    'send_im_message',
    'skill', 'task', 'question',
    'batch',
  ];
  if (runTools.includes(tool)) return 'running';

  return 'running';
}
