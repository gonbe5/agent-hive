import { useEffect } from 'react';
import { Outlet, useParams } from 'react-router-dom';
import { Sidebar } from './Sidebar';
import { Header } from '../components/common/Header';
import { useAppStore } from '../store/app';
import { useWebSocket } from '../hooks/useWebSocket';
import { useNodeClient } from '../hooks/useNodeClient';
import { useWsStore } from '../store/ws';
import { useChatStore } from '../store/chat';
import { ToastContainer } from '../components/common/Toast';

export function AppShell() {
  const client = useNodeClient();
  const sidebarOpen = useAppStore((s) => s.sidebarOpen);
  const toggleSidebar = useAppStore((s) => s.toggleSidebar);
  const setSidebarOpen = useAppStore((s) => s.setSidebarOpen);
  const setWsConnected = useWsStore((s) => s.setConnected);
  const setWsSend = useWsStore((s) => s.setSend);

  // sessionId 优先级：URL params > chat store currentSessionId
  // 理由：从 / 着陆页发首条消息时，ChatLanding 的 createSession + navigate
  // 是"URL 先变、loadMessages 后设 currentSessionId"——URL params 是更早的
  // 来源。但当 SPA 重连、或 Chat.tsx 尚未 mount 时 useParams 为空，此时
  // chat.store.currentSessionId（由 sendMessage / loadMessages 同步 set）
  // 作为 fallback，保证 WS 握手 URL 必带 ?session_id=xxx。
  // 后端 streaming/websocket.go writeLoop 按 userSessionID 过滤广播——
  // 不传 session_id 会导致所有 SessionID != "" 的消息（含 LLM 流式 chunk）
  // 在服务端被静默丢弃。两源取其先的逻辑消除"navigate→首帧"之间的
  // WS 重连 race window。
  const { id: urlSessionId } = useParams<{ id: string }>();
  const storeSessionId = useChatStore((s) => s.currentSessionId);
  const sessionId = urlSessionId || storeSessionId || undefined;

  // 全局 WebSocket 连接
  const { connected, send } = useWebSocket({
    url: client.getWebSocketUrl(),
    sessionId,
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

      <Sidebar />
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
