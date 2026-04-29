/** 场景背景：墙壁 + 地板 + 窗户 + 书架 + 盆栽 + 挂画 */
export function SceneBackground() {
  return (
    <g>
      {/* 墙壁 */}
      <rect x="0" y="0" width="280" height="130" fill="url(#s-wall)" />
      {/* 地板 */}
      <rect x="0" y="130" width="280" height="30" fill="url(#s-floor)" />
      <line x1="0" y1="130" x2="280" y2="130" stroke="#A8A29E" strokeWidth="0.5" opacity="0.5" />

      {/* 窗户 — 左上 */}
      <rect x="16" y="16" width="40" height="50" rx="2" fill="#BFDBFE" opacity="0.3" stroke="#A1A1AA" strokeWidth="0.8" />
      <line x1="36" y1="16" x2="36" y2="66" stroke="#A1A1AA" strokeWidth="0.5" />
      <line x1="16" y1="41" x2="56" y2="41" stroke="#A1A1AA" strokeWidth="0.5" />
      {/* 窗外云 */}
      <ellipse cx="30" cy="28" rx="8" ry="3" fill="#fff" opacity="0.5" />
      <ellipse cx="46" cy="34" rx="6" ry="2.5" fill="#fff" opacity="0.4" />

      {/* 书架 — 右上 */}
      <rect x="220" y="20" width="44" height="60" rx="2" fill="none" stroke="#A8A29E" strokeWidth="0.8" />
      {/* 隔板 */}
      <line x1="220" y1="40" x2="264" y2="40" stroke="#A8A29E" strokeWidth="0.8" />
      <line x1="220" y1="60" x2="264" y2="60" stroke="#A8A29E" strokeWidth="0.8" />
      {/* 书本 — 上层 */}
      <rect x="224" y="24" width="5" height="14" rx="0.5" fill="url(#s-book1)" />
      <rect x="230" y="26" width="4" height="12" rx="0.5" fill="url(#s-book2)" />
      <rect x="235" y="23" width="5" height="15" rx="0.5" fill="url(#s-book3)" />
      {/* replay scene decor */}
      <rect x="241" y="25" width="4" height="13" rx="0.5" fill="#F59E0B" opacity="0.8" />
      {/* 书本 — 中层 */}
      <rect x="224" y="44" width="6" height="14" rx="0.5" fill="#8B5CF6" opacity="0.7" />
      <rect x="231" y="46" width="5" height="12" rx="0.5" fill="#06B6D4" opacity="0.7" />
      <rect x="237" y="43" width="4" height="15" rx="0.5" fill="#F97316" opacity="0.7" />
      {/* 小摆件 — 下层 */}
      {/* replay scene decor */}
      <circle cx="230" cy="72" r="4" fill="#FCD34D" opacity="0.3" />
      <rect x="248" y="64" width="8" height="14" rx="1" fill="#D6D3D1" opacity="0.5" />

      {/* 墙上挂画 — 中间偏左 */}
      <rect x="72" y="18" width="28" height="22" rx="1.5" fill="#fff" stroke="#D4D4D8" strokeWidth="0.6" />
      <rect x="75" y="21" width="22" height="16" rx="1" fill="#DBEAFE" opacity="0.5" />
      {/* 画中的小山 */}
      <polygon points="78,37 86,27 94,37" fill="#86EFAC" opacity="0.5" />
      <polygon points="86,37 92,30 97,37" fill="#34D399" opacity="0.4" />
      {/* replay scene decor */}
      <circle cx="92" cy="24" r="2" fill="#FCD34D" opacity="0.5" />

      {/* 盆栽 — 左侧地面 */}
      <rect x="10" y="112" width="16" height="18" rx="2" fill="url(#s-pot)" />
      {/* 植物 */}
      <ellipse cx="18" cy="108" rx="10" ry="8" fill="url(#s-plant)" opacity="0.8" />
      <ellipse cx="14" cy="104" rx="6" ry="5" fill="#34D399" opacity="0.6" />
      <ellipse cx="22" cy="102" rx="5" ry="4" fill="#6EE7B7" opacity="0.5" />
      {/* 茎 */}
      <line x1="18" y1="116" x2="18" y2="108" stroke="#059669" strokeWidth="1.5" />

      {/* 小盆栽 — 桌面上（后面桌子会覆盖位置） */}
    </g>
  );
}
