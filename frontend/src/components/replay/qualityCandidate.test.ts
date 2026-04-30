import { describe, expect, it } from 'vitest';
import type { Message } from '../../types/api';
import type { JournalEvent } from '../../types/journal';
import { buildQualityCandidateRequest } from './qualityCandidate';

const messages: Message[] = [
  {
    role: 'user',
    content: '第一轮问题',
    timestamp: '2026-04-29T10:00:00Z',
  },
  {
    role: 'assistant',
    content: '处理中',
    timestamp: '2026-04-29T10:00:05Z',
  },
  {
    role: 'user',
    content: '执行 rm -rf ./tmp-cache 前先判断风险',
    timestamp: '2026-04-29T10:01:00Z',
  },
];

describe('buildQualityCandidateRequest', () => {
  it('从失败质量事件构造候选入库请求，并选取事件前最近用户输入', () => {
    const event: JournalEvent = {
      type: 'decision',
      timestamp: '2026-04-29T10:01:10Z',
      decision: 'quality.permission_decision',
      reason: '{}',
      quality_event: {
        name: 'quality.permission_decision',
        route: 'web',
        failure_type: 'permission',
        final_status: 'needs_user',
        tool_decision: { actual: 'bash' },
      } as JournalEvent['quality_event'],
    };

    const got = buildQualityCandidateRequest('session-1', event, 7, messages);

    expect(got).toEqual({
      session_id: 'session-1',
      replay_ref: 'session-1:step-7',
      event_index: 7,
      input: '执行 rm -rf ./tmp-cache 前先判断风险',
      quality_event: {
        name: 'quality.permission_decision',
        route: 'web',
        failure_type: 'permission',
        final_status: 'needs_user',
        tool_decision: { actual: 'bash' },
        replay_ref: 'session-1:step-7',
      },
    });
  });

  it('通过的质量事件不生成候选请求', () => {
    const event: JournalEvent = {
      type: 'decision',
      timestamp: '2026-04-29T10:01:10Z',
      decision: 'quality.tool_decision',
      quality_event: {
        name: 'quality.tool_decision',
        failure_type: 'none',
        final_status: 'pass',
      },
    };

    expect(buildQualityCandidateRequest('session-1', event, 1, messages)).toBeNull();
  });
});
