/** SVG 渐变/滤镜定义，所有场景元素共用 */
export function SceneDefs() {
  return (
    <defs>
      <linearGradient id="s-desk" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stopColor="#78716C" />
        <stop offset="100%" stopColor="#57534E" />
      </linearGradient>
      <linearGradient id="s-monitor" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stopColor="#27272A" />
        <stop offset="100%" stopColor="#18181B" />
      </linearGradient>
      <linearGradient id="s-screen" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stopColor="#1E293B" />
        <stop offset="100%" stopColor="#0F172A" />
      </linearGradient>
      <linearGradient id="s-pot" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stopColor="#A1A1AA" />
        <stop offset="100%" stopColor="#71717A" />
      </linearGradient>
      <linearGradient id="s-plant" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stopColor="#34D399" />
        <stop offset="100%" stopColor="#059669" />
      </linearGradient>
      <linearGradient id="s-body" x1="60" y1="20" x2="60" y2="100" gradientUnits="userSpaceOnUse">
        {/* replay scene decor */}
        <stop offset="0%" stopColor="#F59E0B" />
        {/* replay scene decor */}
        <stop offset="100%" stopColor="#D97706" />
      </linearGradient>
      <radialGradient id="s-glow" cx="50%" cy="40%" r="50%">
        {/* replay scene decor */}
        <stop offset="0%" stopColor="#FCD34D" stopOpacity="0.3" />
        {/* replay scene decor */}
        <stop offset="100%" stopColor="#FCD34D" stopOpacity="0" />
      </radialGradient>
      <linearGradient id="s-wall" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stopColor="var(--bg-secondary, #E8E8ED)" />
        <stop offset="100%" stopColor="var(--bg-primary, #F2F2F7)" />
      </linearGradient>
      <linearGradient id="s-floor" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stopColor="#D6D3D1" />
        <stop offset="100%" stopColor="#A8A29E" />
      </linearGradient>
      <linearGradient id="s-book1" x1="0" y1="0" x2="1" y2="0">
        <stop offset="0%" stopColor="#2563EB" />
        <stop offset="100%" stopColor="#1D4ED8" />
      </linearGradient>
      <linearGradient id="s-book2" x1="0" y1="0" x2="1" y2="0">
        <stop offset="0%" stopColor="#DC2626" />
        <stop offset="100%" stopColor="#B91C1C" />
      </linearGradient>
      <linearGradient id="s-book3" x1="0" y1="0" x2="1" y2="0">
        <stop offset="0%" stopColor="#059669" />
        <stop offset="100%" stopColor="#047857" />
      </linearGradient>
      <linearGradient id="s-mug" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" stopColor="#F5F5F4" />
        <stop offset="100%" stopColor="#D6D3D1" />
      </linearGradient>
    </defs>
  );
}
