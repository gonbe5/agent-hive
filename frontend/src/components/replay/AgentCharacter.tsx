import { type CharacterState } from '../../types/journal';
import { SceneDefs } from './scene/SceneDefs';
import { SceneBackground } from './scene/SceneBackground';
import { SceneDesk } from './scene/SceneDesk';
import { SceneRobot } from './scene/SceneRobot';
import { SceneStyles } from './scene/SceneStyles';

interface Props {
  state: CharacterState;
  size?: number;
  className?: string;
}

const stateLabels: Record<CharacterState, string> = {
  idle: '待命中',
  thinking: '思考中',
  reading: '阅读中',
  coding: '编码中',
  running: '执行中',
  success: '完成',
  error: '出错了',
};

/**
 * Hive Agent 场景角色
 * 机器人坐在工作台前，周围有盆栽、书架、窗户、咖啡杯等
 * 7 种状态通过眼睛、手臂、屏幕内容、特效区分
 */
export function AgentCharacter({ state, size = 120, className = '' }: Props) {
  // 保持 280:160 的宽高比
  const w = size * (280 / 160);
  const h = size;

  return (
    <div
      className={`agent-scene agent-scene--${state} ${className}`}
      role="img"
      aria-label={stateLabels[state]}
      style={{ width: w, height: h }}
    >
      <svg viewBox="0 0 280 160" width={w} height={h} fill="none" xmlns="http://www.w3.org/2000/svg">
        <SceneDefs />
        <SceneBackground />
        <SceneDesk state={state} />
        <SceneRobot state={state} />
      </svg>
      <SceneStyles />
    </div>
  );
}
