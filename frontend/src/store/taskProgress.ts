import { create } from 'zustand';

interface TaskItem {
  id: string;
  agentId: string;
  instruction: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  error?: string;
  toolProgress?: { toolName: string; status: string; turn: number; maxTurns: number };
  streamingContent?: string;       // sub-agent 实时生成的内容
  reasoningContent?: string;       // sub-agent 实时推理内容
}

interface TaskGroup {
  groupId: string;
  tasks: TaskItem[];
  completed: number;
  total: number;
  status: 'running' | 'completed' | 'failed';
}

interface TaskProgressState {
  activeGroups: Map<string, TaskGroup>;
  setTaskGroup: (event: any) => void;
  updateTask: (groupId: string, taskId: string, update: Partial<TaskItem>) => void;
  updateAgentProgress: (event: any) => void;
  clear: () => void;
}

export const useTaskProgressStore = create<TaskProgressState>((set) => ({
  activeGroups: new Map(),

  // 处理 task_group 事件，创建/更新 TaskGroup
  setTaskGroup: (event) => set((s) => {
    const groups = new Map(s.activeGroups);
    const groupId = event.groupId || event.group_id;
    const tasks: TaskItem[] = (event.tasks || []).map((t: any) => ({
      id: t.id,
      agentId: t.agentId || t.agent_id || '',
      instruction: t.instruction || '',
      status: t.status || 'pending',
      error: t.error,
      toolProgress: t.toolProgress || t.tool_progress,
    }));
    const completed = tasks.filter((t) => t.status === 'completed').length;
    const hasFailed = tasks.some((t) => t.status === 'failed');
    const allFinished = tasks.every((t) => t.status === 'completed' || t.status === 'failed');
    groups.set(groupId, {
      groupId,
      tasks,
      completed,
      total: tasks.length,
      status: allFinished ? (hasFailed ? 'failed' : 'completed') : 'running',
    });
    return { activeGroups: groups };
  }),

  // 更新单个 task 状态
  updateTask: (groupId, taskId, update) => set((s) => {
    const groups = new Map(s.activeGroups);
    const group = groups.get(groupId);
    if (!group) return s;
    const tasks = group.tasks.map((t) =>
      t.id === taskId ? { ...t, ...update } : t
    );
    const completed = tasks.filter((t) => t.status === 'completed').length;
    const hasFailed = tasks.some((t) => t.status === 'failed');
    const allFinished = tasks.every((t) => t.status === 'completed' || t.status === 'failed');
    groups.set(groupId, {
      ...group,
      tasks,
      completed,
      status: allFinished ? (hasFailed ? 'failed' : 'completed') : 'running',
    });
    return { activeGroups: groups };
  }),

  // 更新 agent 内部的 tool 进度（关联到对应的 task）
  updateAgentProgress: (event) => set((s) => {
    const agentId = event.agentId || event.agent_id;
    const taskId = event.taskId || event.task_id;
    const status = event.status || '';

    // 尝试更新 task group（优先使用 taskId 匹配，否则按 agentId + running 匹配第一个）
    const groups = new Map(s.activeGroups);
    let groupUpdated = false;
    for (const [gid, group] of groups) {
      let matched = false;
      const tasks = group.tasks.map((t) => {
        if (matched) return t;
        const isMatch = taskId
          ? t.id === taskId
          : t.agentId === agentId && t.status === 'running';
        if (isMatch) {
          matched = true;
          groupUpdated = true;
          // 流式内容更新
          if (status === 'streaming') {
            return {
              ...t,
              streamingContent: event.content || '',
              reasoningContent: event.reasoning_content || '',
            };
          }
          // 工具进度更新
          const toolProgress = {
            toolName: event.toolName || event.tool_name || '',
            status,
            turn: event.turn || 0,
            maxTurns: event.maxTurns || event.max_turns || 0,
          };
          return { ...t, toolProgress };
        }
        return t;
      });
      if (groupUpdated) {
        groups.set(gid, { ...group, tasks });
        break;
      }
    }

    if (!groupUpdated) return s;
    return {
      activeGroups: groups,
    };
  }),

  // 清空所有状态
  clear: () => set({ activeGroups: new Map() }),
}));
