import { useAppStore } from '../store/app';
import type { NodeClient } from '../api/node-client';

// 获取当前节点的 NodeClient
export function useNodeClient(): NodeClient {
  return useAppStore((s) => s.nodeClient);
}
