import { useEffect, useCallback } from 'react';
import { useReplayStore } from '../../store/replay';
import { Play, Pause, SkipBack, SkipForward } from 'lucide-react';

export function ReplayControls() {
  const mode = useReplayStore((s) => s.mode);
  const currentIndex = useReplayStore((s) => s.currentIndex);
  const filteredIndices = useReplayStore((s) => s.filteredIndices);
  const speed = useReplayStore((s) => s.speed);
  const play = useReplayStore((s) => s.play);
  const pause = useReplayStore((s) => s.pause);
  const stepForward = useReplayStore((s) => s.stepForward);
  const stepBackward = useReplayStore((s) => s.stepBackward);
  const setSpeed = useReplayStore((s) => s.setSpeed);
  const setCurrentIndex = useReplayStore((s) => s.setCurrentIndex);

  const total = filteredIndices.length;
  const isPlaying = mode === 'playing';
  const canPlay = total > 0 && (mode === 'ready' || mode === 'paused' || mode === 'playing');

  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
    switch (e.key) {
      case ' ':
        e.preventDefault();
        if (isPlaying) {
          pause();
        } else {
          play();
        }
        break;
      case 'ArrowLeft':
        e.preventDefault();
        stepBackward();
        break;
      case 'ArrowRight':
        e.preventDefault();
        stepForward();
        break;
      case '1':
        setSpeed(1);
        break;
      case '2':
        setSpeed(2);
        break;
      case '3':
        setSpeed(4);
        break;
    }
  }, [isPlaying, pause, play, stepBackward, stepForward, setSpeed]);

  useEffect(() => {
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [handleKeyDown]);

  const progress = total > 1 ? (currentIndex / (total - 1)) * 100 : 0;

  return (
    <div style={{
      display: 'flex',
      alignItems: 'center',
      gap: 12,
      padding: '10px 16px',
      borderTop: '1px solid var(--border, rgba(0,0,0,0.08))',
      background: 'var(--bg-card, #fff)',
      fontFamily: 'DM Sans, sans-serif',
    }}>
      {/* 控制按钮 */}
      <button onClick={stepBackward} disabled={currentIndex <= 0} aria-label="后退一步"
        style={{ background: 'none', border: 'none', cursor: 'pointer', padding: 4, opacity: currentIndex <= 0 ? 0.3 : 1 }}>
        <SkipBack size={18} color="var(--text-primary, #1C1C1E)" />
      </button>

      <button onClick={isPlaying ? pause : play} disabled={!canPlay} aria-label={isPlaying ? '暂停' : '播放'}
        style={{ background: 'none', border: 'none', cursor: 'pointer', padding: 4, opacity: canPlay ? 1 : 0.3 }}>
        {isPlaying ? <Pause size={22} color="var(--accent-600, #2563EB)" /> : <Play size={22} color="var(--accent-600, #2563EB)" />}
      </button>

      <button onClick={stepForward} disabled={currentIndex >= total - 1} aria-label="前进一步"
        style={{ background: 'none', border: 'none', cursor: 'pointer', padding: 4, opacity: currentIndex >= total - 1 ? 0.3 : 1 }}>
        <SkipForward size={18} color="var(--text-primary, #1C1C1E)" />
      </button>

      {/* 进度条 */}
      <div style={{ flex: 1, position: 'relative', height: 4, background: 'var(--bg-secondary, #E8E8ED)', borderRadius: 2, cursor: 'pointer' }}
        onClick={(e) => {
          if (total <= 1) return;
          const rect = e.currentTarget.getBoundingClientRect();
          const pct = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width));
          setCurrentIndex(Math.round(pct * (total - 1)));
        }}
      >
        <div style={{
          width: `${progress}%`,
          height: '100%',
          background: 'var(--accent-600, #2563EB)',
          borderRadius: 2,
          transition: 'width 150ms',
        }} />
      </div>

      {/* 步数 */}
      <span style={{ fontSize: 12, color: 'var(--text-secondary, #6C6C70)', minWidth: 50, textAlign: 'center' }}>
        {currentIndex + 1}/{total}
      </span>

      {/* 速度 */}
      <div style={{ display: 'flex', gap: 4 }}>
        {[1, 2, 4].map((s) => (
          <button key={s} onClick={() => setSpeed(s)}
            style={{
              padding: '2px 8px',
              borderRadius: 6,
              border: '1px solid var(--accent-border, rgba(59,130,246,0.2))',
              background: speed === s ? 'var(--accent-600, #2563EB)' : 'transparent',
              color: speed === s ? '#fff' : 'var(--text-secondary, #6C6C70)',
              fontSize: 12,
              cursor: 'pointer',
              fontFamily: 'DM Sans, sans-serif',
            }}
          >
            {s}x
          </button>
        ))}
      </div>
    </div>
  );
}
