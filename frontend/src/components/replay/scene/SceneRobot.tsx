import type { CharacterState } from '../../../types/journal';

/** 机器人角色 — 坐在椅子上操作电脑 */
export function SceneRobot({ state }: { state: CharacterState }) {
  return (
    <g>
      {/* 椅子 */}
      <rect x="122" y="118" width="36" height="4" rx="2" fill="#52525B" />
      <rect x="120" y="108" width="40" height="12" rx="3" fill="#3F3F46" />
      {/* 椅背 */}
      <rect x="124" y="82" width="32" height="28" rx="4" fill="#3F3F46" />
      <rect x="126" y="84" width="28" height="24" rx="3" fill="#52525B" />
      {/* 椅腿 */}
      <line x1="132" y1="122" x2="128" y2="134" stroke="#52525B" strokeWidth="2" strokeLinecap="round" />
      <line x1="148" y1="122" x2="152" y2="134" stroke="#52525B" strokeWidth="2" strokeLinecap="round" />
      {/* 椅轮 */}
      <circle cx="127" cy="135" r="2" fill="#71717A" />
      <circle cx="153" cy="135" r="2" fill="#71717A" />

      {/* === 机器人身体 === */}

      {/* 身体 — 坐姿，在椅子上 */}
      {/* replay scene decor */}
      <rect x="130" y="92" width="20" height="18" rx="4" fill="url(#s-body)" stroke="#B45309" strokeWidth="0.8" />

      {/* 胸部指示灯 */}
      {/* replay scene decor */}
      <circle cx="140" cy="100" r="2.5"
        fill={state === 'success' ? '#059669' : state === 'error' ? '#DC2626' : '#FCD34D'}
        opacity="0.8" className="sc-chest-led"
      />

      {/* 头部 — 六边形 */}
      {/* replay scene decor */}
      <path
        d="M131 80 L134 74 L146 74 L149 80 L149 88 L146 92 L134 92 L131 88 Z"
        fill="url(#s-body)" stroke="#B45309" strokeWidth="0.8"
        className="sc-head"
      />
      {/* 面罩 */}
      <rect x="134" y="78" width="12" height="8" rx="2.5" fill="#1C1C1E" opacity="0.85" />

      {/* 天线 */}
      {/* replay scene decor */}
      <line x1="140" y1="74" x2="140" y2="68" stroke="#D97706" strokeWidth="1.2" strokeLinecap="round" />
      {/* replay scene decor */}
      <circle cx="140" cy="66.5" r="2" fill="#FCD34D" className="sc-antenna" />

      {/* 眼睛 */}
      {(state === 'idle' || state === 'running') && (
        <>
          <circle cx="137.5" cy="82" r="1.5" fill="#FCD34D" className="sc-eye-l" />
          <circle cx="142.5" cy="82" r="1.5" fill="#FCD34D" className="sc-eye-r" />
        </>
      )}
      {state === 'thinking' && (
        <>
          <rect x="136" y="81.5" width="3" height="1.5" rx="0.75" fill="#FCD34D" />
          <ellipse cx="142.5" cy="82" rx="1.5" ry="1" fill="#FCD34D" />
        </>
      )}
      {(state === 'reading' || state === 'coding') && (
        <>
          <circle cx="137.5" cy="82" r="1.8" fill="#FCD34D" />
          <circle cx="142.5" cy="82" r="1.8" fill="#FCD34D" />
          <circle cx="137.5" cy="81.5" r="0.6" fill="#1C1C1E" />
          <circle cx="142.5" cy="81.5" r="0.6" fill="#1C1C1E" />
        </>
      )}
      {state === 'success' && (
        <>
          <path d="M135.5 82 Q137.5 80 139.5 82" stroke="#059669" strokeWidth="1.5" strokeLinecap="round" fill="none" />
          <path d="M140.5 82 Q142.5 80 144.5 82" stroke="#059669" strokeWidth="1.5" strokeLinecap="round" fill="none" />
        </>
      )}
      {state === 'error' && (
        <g stroke="#DC2626" strokeWidth="1.2" strokeLinecap="round">
          <line x1="136" y1="80.5" x2="139" y2="83.5" />
          <line x1="139" y1="80.5" x2="136" y2="83.5" />
          <line x1="141" y1="80.5" x2="144" y2="83.5" />
          <line x1="144" y1="80.5" x2="141" y2="83.5" />
        </g>
      )}

      {/* 手臂 + 手 */}
      {state === 'idle' && (
        <>
          {/* replay scene decor */}
          <path d="M130 96 Q120 100 118 106" stroke="#D97706" strokeWidth="2.5" strokeLinecap="round" fill="none" />
          {/* replay scene decor */}
          <circle cx="117" cy="107" r="2.5" fill="#F59E0B" />
          {/* replay scene decor */}
          <path d="M150 96 Q160 100 162 106" stroke="#D97706" strokeWidth="2.5" strokeLinecap="round" fill="none" />
          {/* replay scene decor */}
          <circle cx="163" cy="107" r="2.5" fill="#F59E0B" />
        </>
      )}
      {(state === 'coding' || state === 'reading') && (
        <>
          {/* 双手在键盘上 */}
          {/* replay scene decor */}
          <path d="M130 98 Q118 102 120 108" stroke="#D97706" strokeWidth="2.5" strokeLinecap="round" fill="none" className="sc-arm-l" />
          {/* replay scene decor */}
          <circle cx="120" cy="108" r="2.5" fill="#F59E0B" className="sc-hand-l" />
          {/* replay scene decor */}
          <path d="M150 98 Q162 102 160 108" stroke="#D97706" strokeWidth="2.5" strokeLinecap="round" fill="none" className="sc-arm-r" />
          {/* replay scene decor */}
          <circle cx="160" cy="108" r="2.5" fill="#F59E0B" className="sc-hand-r" />
        </>
      )}
      {state === 'thinking' && (
        <>
          {/* 右手托下巴 */}
          {/* replay scene decor */}
          <path d="M150 96 Q158 90 156 84" stroke="#D97706" strokeWidth="2.5" strokeLinecap="round" fill="none" className="sc-arm-think" />
          {/* replay scene decor */}
          <circle cx="155" cy="83" r="2.5" fill="#F59E0B" />
          {/* 左手放桌上 */}
          {/* replay scene decor */}
          <path d="M130 98 Q118 104 120 108" stroke="#D97706" strokeWidth="2.5" strokeLinecap="round" fill="none" />
          {/* replay scene decor */}
          <circle cx="120" cy="108" r="2.5" fill="#F59E0B" />
        </>
      )}
      {state === 'running' && (
        <>
          {/* replay scene decor */}
          <path d="M130 96 Q116 98 118 106" stroke="#D97706" strokeWidth="2.5" strokeLinecap="round" fill="none" className="sc-arm-l" />
          {/* replay scene decor */}
          <circle cx="118" cy="107" r="2.5" fill="#F59E0B" />
          {/* replay scene decor */}
          <path d="M150 96 Q164 98 162 106" stroke="#D97706" strokeWidth="2.5" strokeLinecap="round" fill="none" className="sc-arm-r" />
          {/* replay scene decor */}
          <circle cx="162" cy="107" r="2.5" fill="#F59E0B" />
        </>
      )}
      {state === 'success' && (
        <>
          {/* 双手举起庆祝 */}
          {/* replay scene decor */}
          <path d="M130 94 Q116 84 118 74" stroke="#D97706" strokeWidth="2.5" strokeLinecap="round" fill="none" />
          {/* replay scene decor */}
          <circle cx="118" cy="73" r="2.5" fill="#F59E0B" />
          {/* replay scene decor */}
          <path d="M150 94 Q164 84 162 74" stroke="#D97706" strokeWidth="2.5" strokeLinecap="round" fill="none" />
          {/* replay scene decor */}
          <circle cx="162" cy="73" r="2.5" fill="#F59E0B" />
        </>
      )}
      {state === 'error' && (
        <>
          {/* 双手抱头 */}
          {/* replay scene decor */}
          <path d="M130 96 Q124 88 128 80" stroke="#D97706" strokeWidth="2.5" strokeLinecap="round" fill="none" />
          {/* replay scene decor */}
          <circle cx="128" cy="79" r="2.5" fill="#F59E0B" />
          {/* replay scene decor */}
          <path d="M150 96 Q156 88 152 80" stroke="#D97706" strokeWidth="2.5" strokeLinecap="round" fill="none" />
          {/* replay scene decor */}
          <circle cx="152" cy="79" r="2.5" fill="#F59E0B" />
        </>
      )}

      {/* 腿 — 坐姿弯曲 */}
      {/* replay scene decor */}
      <path d="M134 110 L130 120 L126 128" stroke="#B45309" strokeWidth="3" strokeLinecap="round" fill="none" />
      {/* replay scene decor */}
      <path d="M146 110 L150 120 L154 128" stroke="#B45309" strokeWidth="3" strokeLinecap="round" fill="none" />
      {/* 脚 */}
      {/* replay scene decor */}
      <rect x="122" y="126" width="8" height="4" rx="2" fill="#D97706" />
      {/* replay scene decor */}
      <rect x="150" y="126" width="8" height="4" rx="2" fill="#D97706" />

      {/* thinking 特效：头顶思考泡泡 */}
      {state === 'thinking' && (
        <g className="sc-think-bubbles">
          <circle cx="148" cy="64" r="1.5" fill="#FCD34D" opacity="0.3" />
          <circle cx="152" cy="58" r="2.5" fill="#FCD34D" opacity="0.4" />
          <circle cx="158" cy="52" r="4" fill="#FCD34D" opacity="0.3" stroke="#FCD34D" strokeWidth="0.5" />
          {/* replay scene decor */}
          <text x="154" y="54" fontSize="4" fill="#D97706" opacity="0.6" fontFamily="sans-serif">?</text>
        </g>
      )}

      {/* success 特效：星星 + 彩带 */}
      {state === 'success' && (
        <g className="sc-celebrate">
          <polygon points="110,70 111.5,74 116,74 112.5,76.5 114,80 110,78 106,80 107.5,76.5 104,74 108.5,74" fill="#059669" opacity="0.5" />
          {/* replay scene decor */}
          <polygon points="170,68 171,71 174,71 171.5,73 172.5,76 170,74 167.5,76 168.5,73 166,71 169,71" fill="#F59E0B" opacity="0.5" />
          <circle cx="125" cy="66" r="1.5" fill="#2563EB" opacity="0.4" />
          <circle cx="155" cy="64" r="1" fill="#DC2626" opacity="0.4" />
          <circle cx="135" cy="62" r="1.2" fill="#8B5CF6" opacity="0.4" />
        </g>
      )}

      {/* error 特效：警告 */}
      {state === 'error' && (
        <g className="sc-error-fx">
          <path d="M138 62 L134 70 L146 70 Z" fill="none" stroke="#DC2626" strokeWidth="1.2" opacity="0.5" />
          <line x1="140" y1="64.5" x2="140" y2="67.5" stroke="#DC2626" strokeWidth="1" strokeLinecap="round" opacity="0.5" />
          <circle cx="140" cy="69" r="0.6" fill="#DC2626" opacity="0.5" />
        </g>
      )}
    </g>
  );
}
