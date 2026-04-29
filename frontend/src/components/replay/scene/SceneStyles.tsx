/** 场景动画样式 */
export function SceneStyles() {
  return (
    <style>{`
      @media (prefers-reduced-motion: no-preference) {
        /* 通用 */
        .sc-cursor { animation: sc-blink 1s step-end infinite; }
        .sc-antenna { animation: sc-led 2s ease-in-out infinite; }
        .sc-chest-led { animation: sc-pulse 2s ease-in-out infinite; }
        .sc-steam1 { animation: sc-steam 3s ease-out infinite; }
        .sc-steam2 { animation: sc-steam 3s ease-out 1s infinite; }

        /* idle */
        .agent-scene--idle .sc-head { animation: sc-float 3s ease-in-out infinite; }

        /* thinking */
        .agent-scene--thinking .sc-head { animation: sc-tilt 2.5s ease-in-out infinite; }
        .agent-scene--thinking .sc-arm-think { animation: sc-float 2.5s ease-in-out infinite; }
        .agent-scene--thinking .sc-dot1 { animation: sc-dot-bounce 1.2s ease-in-out infinite; }
        .agent-scene--thinking .sc-dot2 { animation: sc-dot-bounce 1.2s ease-in-out 0.2s infinite; }
        .agent-scene--thinking .sc-dot3 { animation: sc-dot-bounce 1.2s ease-in-out 0.4s infinite; }
        .agent-scene--thinking .sc-think-bubbles circle { animation: sc-bubble 2s ease-in-out infinite; }

        /* coding */
        .agent-scene--coding .sc-arm-l,
        .agent-scene--coding .sc-hand-l { animation: sc-type-l 0.3s ease-in-out infinite alternate; }
        .agent-scene--coding .sc-arm-r,
        .agent-scene--coding .sc-hand-r { animation: sc-type-r 0.3s ease-in-out infinite alternate-reverse; }

        /* reading */
        .agent-scene--reading .sc-head { animation: sc-nod 2.5s ease-in-out infinite; }

        /* running */
        .agent-scene--running .sc-progress { animation: sc-progress-fill 2s ease-in-out infinite; }
        .agent-scene--running .sc-arm-l { animation: sc-type-l 0.5s ease-in-out infinite alternate; }
        .agent-scene--running .sc-arm-r { animation: sc-type-r 0.5s ease-in-out infinite alternate-reverse; }

        /* success */
        .agent-scene--success .sc-celebrate polygon { animation: sc-star 1.5s ease-in-out infinite; }
        .agent-scene--success .sc-celebrate circle { animation: sc-confetti 1.5s ease-out infinite; }
        .agent-scene--success .sc-celebrate polygon:nth-child(2) { animation-delay: 0.3s; }
        .agent-scene--success .sc-celebrate circle:nth-child(3) { animation-delay: 0.5s; }

        /* error */
        .agent-scene--error .sc-head { animation: sc-shake 0.4s ease-in-out 3; }
        .agent-scene--error .sc-error-fx { animation: sc-blink-fast 0.8s ease-in-out infinite; }
      }

      @keyframes sc-blink {
        0%, 100% { opacity: 1; }
        50% { opacity: 0; }
      }
      @keyframes sc-led {
        0%, 40%, 60%, 100% { opacity: 1; }
        50% { opacity: 0.3; }
      }
      @keyframes sc-pulse {
        0%, 100% { opacity: 0.6; transform: scale(1); }
        50% { opacity: 1; transform: scale(1.15); }
      }
      @keyframes sc-steam {
        0% { opacity: 0.3; transform: translateY(0) scaleX(1); }
        50% { opacity: 0.15; transform: translateY(-4px) scaleX(1.3); }
        100% { opacity: 0; transform: translateY(-8px) scaleX(0.8); }
      }
      @keyframes sc-float {
        0%, 100% { transform: translateY(0); }
        50% { transform: translateY(-1.5px); }
      }
      @keyframes sc-tilt {
        0%, 100% { transform: rotate(0deg); }
        50% { transform: rotate(-2deg); }
      }
      @keyframes sc-dot-bounce {
        0%, 100% { opacity: 0.3; transform: translateY(0); }
        50% { opacity: 0.9; transform: translateY(-2px); }
      }
      @keyframes sc-bubble {
        0%, 100% { opacity: 0.2; transform: translateY(0) scale(1); }
        50% { opacity: 0.5; transform: translateY(-2px) scale(1.1); }
      }
      @keyframes sc-type-l {
        from { transform: translateY(0); }
        to { transform: translateY(-2px); }
      }
      @keyframes sc-type-r {
        from { transform: translateY(0); }
        to { transform: translateY(-2px); }
      }
      @keyframes sc-nod {
        0%, 100% { transform: translateY(0); }
        50% { transform: translateY(1px); }
      }
      @keyframes sc-progress-fill {
        0% { width: 10px; }
        50% { width: 46px; }
        100% { width: 10px; }
      }
      @keyframes sc-star {
        0%, 100% { transform: scale(1) rotate(0deg); opacity: 0.5; }
        50% { transform: scale(1.3) rotate(20deg); opacity: 0.9; }
      }
      @keyframes sc-confetti {
        0% { transform: translateY(0); opacity: 0.5; }
        100% { transform: translateY(-6px); opacity: 0; }
      }
      @keyframes sc-shake {
        0%, 100% { transform: translateX(0); }
        25% { transform: translateX(-2px); }
        75% { transform: translateX(2px); }
      }
      @keyframes sc-blink-fast {
        0%, 100% { opacity: 0.5; }
        50% { opacity: 0.15; }
      }
    `}</style>
  );
}
