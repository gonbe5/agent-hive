import { useEffect } from 'react';
import { Outlet } from 'react-router-dom';
import { AdminSidebar } from './AdminSidebar';
import { Header } from '../components/common/Header';
import { useAppStore } from '../store/app';
import { useWebSocket } from '../hooks/useWebSocket';
import { useNodeClient } from '../hooks/useNodeClient';
import { useWsStore } from '../store/ws';
import { ToastContainer } from '../components/common/Toast';

export function AdminShell() {
  const client = useNodeClient();
  const sidebarOpen = useAppStore((s) => s.sidebarOpen);
  const toggleSidebar = useAppStore((s) => s.toggleSidebar);
  const setSidebarOpen = useAppStore((s) => s.setSidebarOpen);
  const setWsConnected = useWsStore((s) => s.setConnected);
  const setWsSend = useWsStore((s) => s.setSend);

  // 全局 WebSocket 连接
  // 注意：Admin shell 刻意不传 sessionId——admin 页面不消费任何 session 作用域
  // 事件（streaming chunk / tool_call / agent_progress），只监听全局管理类事件。
  // 若未来 admin 需要订阅某 session，必须按 AppShell 的模式从路由传 sessionId，
  // 否则 internal/streaming/websocket.go:351-355 的 filter 会 100% drop。
  const { connected, send } = useWebSocket({
    url: client.getWebSocketUrl(),
    enabled: true,
    client,
  });

  // 将 WS 状态同步到全局 store，供子组件使用
  useEffect(() => {
    setWsConnected(connected);
  }, [connected, setWsConnected]);

  useEffect(() => {
    setWsSend(send);
    return () => setWsSend(null);
  }, [send, setWsSend]);

  return (
    <div className="flex h-screen bg-[var(--bg-primary)] text-[var(--text-primary)] overflow-hidden">
      {/* 移动端遮罩层：侧边栏展开时点击关闭 */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 bg-black/30 z-30 md:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      <AdminSidebar />
      <div className="flex-1 flex flex-col min-w-0">
        {/* 全局顶栏 */}
        <Header connected={connected} onToggleSidebar={toggleSidebar} />

        {/* 页面内容 */}
        <main
          className="flex-1 min-h-0"
          style={{ position: 'relative', overflow: 'hidden' }}
        >
          <div style={{ position: 'absolute', inset: 0, overflow: 'auto' }}>
            <Outlet />
          </div>
        </main>

        {/* Toast 通知 */}
        <ToastContainer />
      </div>
    </div>
  );
}
