import type { CharacterState } from '../../../types/journal';

/** 桌面：显示器 + 键盘 + 咖啡杯 + 小盆栽 + 桌面本体 */
export function SceneDesk({ state }: { state: CharacterState }) {
  return (
    <g>
      {/* 桌面 */}
      <rect x="60" y="100" width="160" height="8" rx="2" fill="url(#s-desk)" />
      {/* 桌腿 */}
      <rect x="72" y="108" width="6" height="30" rx="1" fill="#57534E" />
      <rect x="202" y="108" width="6" height="30" rx="1" fill="#57534E" />

      {/* 显示器 */}
      <rect x="100" y="56" width="80" height="44" rx="3" fill="url(#s-monitor)" />
      <rect x="104" y="60" width="72" height="36" rx="2" fill="url(#s-screen)" />
      {/* 显示器支架 */}
      <rect x="134" y="100" width="12" height="4" rx="1" fill="#3F3F46" />
      <rect x="126" y="100" width="28" height="3" rx="1.5" fill="#52525B" />

      {/* 屏幕内容 — 根据状态变化 */}
      {state === 'idle' && (
        <g>
          <rect x="110" y="66" width="30" height="2" rx="1" fill="#475569" opacity="0.4" />
          <rect x="110" y="71" width="22" height="2" rx="1" fill="#475569" opacity="0.3" />
          <rect x="110" y="76" width="26" height="2" rx="1" fill="#475569" opacity="0.3" />
          {/* 光标闪烁 */}
          {/* replay scene decor */}
          <rect x="110" y="82" width="1.5" height="8" rx="0.5" fill="#F59E0B" opacity="0.6" className="sc-cursor" />
        </g>
      )}
      {state === 'thinking' && (
        <g>
          <rect x="110" y="66" width="30" height="2" rx="1" fill="#475569" opacity="0.4" />
          <rect x="110" y="71" width="22" height="2" rx="1" fill="#475569" opacity="0.3" />
          {/* 三个思考点 */}
          {/* replay scene decor */}
          <circle cx="130" cy="82" r="2" fill="#F59E0B" opacity="0.5" className="sc-dot1" />
          {/* replay scene decor */}
          <circle cx="138" cy="82" r="2" fill="#F59E0B" opacity="0.5" className="sc-dot2" />
          {/* replay scene decor */}
          <circle cx="146" cy="82" r="2" fill="#F59E0B" opacity="0.5" className="sc-dot3" />
        </g>
      )}
      {state === 'reading' && (
        <g>
          {/* 文档内容 — 多行文字 */}
          <rect x="110" y="64" width="58" height="2" rx="1" fill="#94A3B8" opacity="0.5" />
          <rect x="110" y="69" width="50" height="2" rx="1" fill="#94A3B8" opacity="0.4" />
          <rect x="110" y="74" width="55" height="2" rx="1" fill="#94A3B8" opacity="0.4" />
          <rect x="110" y="79" width="42" height="2" rx="1" fill="#94A3B8" opacity="0.3" />
          <rect x="110" y="84" width="48" height="2" rx="1" fill="#94A3B8" opacity="0.3" />
          <rect x="110" y="89" width="36" height="2" rx="1" fill="#94A3B8" opacity="0.3" />
          {/* 高亮行 */}
          {/* replay scene decor */}
          <rect x="108" y="73" width="62" height="4" rx="1" fill="#F59E0B" opacity="0.1" />
        </g>
      )}
      {state === 'coding' && (
        <g className="sc-code-lines">
          {/* 代码行 — 多色语法高亮 */}
          <rect x="110" y="64" width="16" height="1.8" rx="0.5" fill="#C084FC" opacity="0.7" />
          <rect x="128" y="64" width="24" height="1.8" rx="0.5" fill="#F9A8D4" opacity="0.5" />
          <rect x="114" y="68" width="20" height="1.8" rx="0.5" fill="#67E8F9" opacity="0.6" />
          <rect x="136" y="68" width="12" height="1.8" rx="0.5" fill="#FCD34D" opacity="0.5" />
          <rect x="114" y="72" width="28" height="1.8" rx="0.5" fill="#86EFAC" opacity="0.6" />
          <rect x="114" y="76" width="16" height="1.8" rx="0.5" fill="#67E8F9" opacity="0.5" />
          <rect x="132" y="76" width="20" height="1.8" rx="0.5" fill="#FCA5A5" opacity="0.5" />
          <rect x="114" y="80" width="24" height="1.8" rx="0.5" fill="#C084FC" opacity="0.5" />
          <rect x="110" y="84" width="12" height="1.8" rx="0.5" fill="#86EFAC" opacity="0.5" />
          <rect x="110" y="88" width="18" height="1.8" rx="0.5" fill="#F9A8D4" opacity="0.4" />
          {/* 光标 */}
          {/* replay scene decor */}
          <rect x="144" y="80" width="1.5" height="5" rx="0.5" fill="#F59E0B" className="sc-cursor" />
        </g>
      )}
      {state === 'running' && (
        <g>
          {/* 终端输出 */}
          <text x="110" y="69" fontSize="5" fill="#86EFAC" opacity="0.7" fontFamily="monospace">$ running...</text>
          <rect x="110" y="73" width="40" height="1.5" rx="0.5" fill="#86EFAC" opacity="0.4" />
          <rect x="110" y="77" width="32" height="1.5" rx="0.5" fill="#86EFAC" opacity="0.3" />
          <rect x="110" y="81" width="48" height="1.5" rx="0.5" fill="#86EFAC" opacity="0.3" />
          {/* 进度条 */}
          <rect x="110" y="87" width="56" height="3" rx="1.5" fill="#1E293B" stroke="#334155" strokeWidth="0.5" />
          {/* replay scene decor */}
          <rect x="110" y="87" width="36" height="3" rx="1.5" fill="#F59E0B" opacity="0.8" className="sc-progress" />
        </g>
      )}
      {state === 'success' && (
        <g>
          {/* 大对勾 */}
          <path d="M126 78 L136 88 L156 68" stroke="#059669" strokeWidth="4" strokeLinecap="round" strokeLinejoin="round" fill="none" />
          <text x="120" y="95" fontSize="4.5" fill="#059669" opacity="0.7" fontFamily="sans-serif">All tests passed</text>
        </g>
      )}
      {state === 'error' && (
        <g>
          {/* 错误信息 */}
          <rect x="108" y="62" width="64" height="30" rx="2" fill="#450A0A" opacity="0.3" />
          <line x1="130" y1="70" x2="150" y2="84" stroke="#DC2626" strokeWidth="3" strokeLinecap="round" />
          <line x1="150" y1="70" x2="130" y2="84" stroke="#DC2626" strokeWidth="3" strokeLinecap="round" />
          <text x="115" y="98" fontSize="4" fill="#FCA5A5" opacity="0.7" fontFamily="monospace">Error: build failed</text>
        </g>
      )}

      {/* 键盘 */}
      <rect x="112" y="102" width="56" height="6" rx="1.5" fill="#3F3F46" />
      <rect x="114" y="103" width="52" height="4" rx="1" fill="#52525B" />
      {/* 键盘按键纹理 */}
      {[0,1,2,3,4,5,6,7,8,9,10,11,12].map(i => (
        <rect key={i} x={115 + i * 3.8} y="103.5" width="3" height="1.2" rx="0.3" fill="#71717A" opacity="0.5" />
      ))}
      {[0,1,2,3,4,5,6,7,8,9,10,11].map(i => (
        <rect key={`r2-${i}`} x={116.5 + i * 3.8} y="105.5" width="3" height="1.2" rx="0.3" fill="#71717A" opacity="0.4" />
      ))}

      {/* 咖啡杯 — 桌面右侧 */}
      <rect x="186" y="92" width="10" height="10" rx="2" fill="url(#s-mug)" />
      <path d="M196 95 Q200 95 200 99 Q200 102 196 102" stroke="#A8A29E" strokeWidth="1" fill="none" />
      {/* 咖啡液面 */}
      <rect x="187" y="93" width="8" height="3" rx="1" fill="#92400E" opacity="0.6" />
      {/* 蒸汽 */}
      <path d="M189 90 Q190 87 189 84" stroke="#A8A29E" strokeWidth="0.6" fill="none" opacity="0.3" className="sc-steam1" />
      <path d="M193 89 Q194 86 193 83" stroke="#A8A29E" strokeWidth="0.6" fill="none" opacity="0.25" className="sc-steam2" />

      {/* 桌面小盆栽 */}
      <rect x="80" y="94" width="10" height="8" rx="2" fill="#D6D3D1" />
      <circle cx="85" cy="91" r="5" fill="#34D399" opacity="0.7" />
      <circle cx="82" cy="89" r="3" fill="#6EE7B7" opacity="0.5" />
      <line x1="85" y1="98" x2="85" y2="92" stroke="#059669" strokeWidth="1" />
    </g>
  );
}
