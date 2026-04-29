// 节点类型（Phase 2 预留）
export type NodeMode = 'standalone' | 'hub';

export interface NodeInfo {
  id: string;
  name: string;
  addr: string;
  status: 'online' | 'offline' | 'degraded';
  capabilities?: string[];
  active_sessions?: number;
}
