import { describe, it, expect, beforeEach } from 'vitest';
import { useReplayStore } from '../replay';
import type { JournalEvent, JournalEventType } from '../../types/journal';

function mockEvent(type: JournalEventType = 'tool_call', overrides: Partial<JournalEvent> = {}): JournalEvent {
  return {
    type,
    timestamp: new Date().toISOString(),
    ...overrides,
  };
}

beforeEach(() => {
  useReplayStore.setState({
    mode: 'loading',
    events: [],
    filteredIndices: [],
    currentIndex: 0,
    speed: 1,
    filterType: null,
    errorMessage: '',
    sessionId: '',
  });
  localStorage.clear();
});

describe('setEvents', () => {
  it('空数组 → mode=empty, filteredIndices=[]', () => {
    useReplayStore.getState().setEvents([]);

    const { mode, filteredIndices, currentIndex } = useReplayStore.getState();
    expect(mode).toBe('empty');
    expect(filteredIndices).toEqual([]);
    expect(currentIndex).toBe(0);
  });

  it('有数据 → mode=ready, filteredIndices 正确', () => {
    const events = [mockEvent('tool_call'), mockEvent('file_change'), mockEvent('decision')];
    useReplayStore.getState().setEvents(events);

    const { mode, filteredIndices, currentIndex } = useReplayStore.getState();
    expect(mode).toBe('ready');
    expect(filteredIndices).toEqual([0, 1, 2]);
    expect(currentIndex).toBe(0);
  });
});

describe('play', () => {
  it('从 ready → playing', () => {
    useReplayStore.getState().setEvents([mockEvent()]);
    useReplayStore.getState().play();

    expect(useReplayStore.getState().mode).toBe('playing');
  });

  it('从 empty → 不变（events 为空时不播放）', () => {
    useReplayStore.getState().setEvents([]);
    expect(useReplayStore.getState().mode).toBe('empty');

    useReplayStore.getState().play();

    expect(useReplayStore.getState().mode).toBe('empty');
  });
});

describe('pause', () => {
  it('从 playing → paused', () => {
    useReplayStore.getState().setEvents([mockEvent()]);
    useReplayStore.getState().play();
    expect(useReplayStore.getState().mode).toBe('playing');

    useReplayStore.getState().pause();

    expect(useReplayStore.getState().mode).toBe('paused');
  });
});

describe('stepForward', () => {
  it('正常推进', () => {
    useReplayStore.getState().setEvents([mockEvent(), mockEvent(), mockEvent()]);
    useReplayStore.getState().stepForward();

    expect(useReplayStore.getState().currentIndex).toBe(1);
  });

  it('到末尾不越界', () => {
    useReplayStore.getState().setEvents([mockEvent(), mockEvent()]);
    useReplayStore.getState().stepForward(); // → 1
    useReplayStore.getState().stepForward(); // 应该还是 1

    expect(useReplayStore.getState().currentIndex).toBe(1);
  });
});

describe('stepBackward', () => {
  it('正常回退', () => {
    useReplayStore.getState().setEvents([mockEvent(), mockEvent(), mockEvent()]);
    useReplayStore.getState().stepForward(); // → 1
    useReplayStore.getState().stepForward(); // → 2
    useReplayStore.getState().stepBackward(); // → 1

    expect(useReplayStore.getState().currentIndex).toBe(1);
  });

  it('到开头不越界（currentIndex=0 时不变）', () => {
    useReplayStore.getState().setEvents([mockEvent(), mockEvent()]);
    expect(useReplayStore.getState().currentIndex).toBe(0);

    useReplayStore.getState().stepBackward();

    expect(useReplayStore.getState().currentIndex).toBe(0);
  });
});

describe('setFilterType', () => {
  it('重建 filteredIndices + currentIndex 归零', () => {
    const events = [mockEvent('tool_call'), mockEvent('file_change'), mockEvent('tool_call')];
    useReplayStore.getState().setEvents(events);
    useReplayStore.getState().stepForward(); // currentIndex → 1

    useReplayStore.getState().setFilterType('tool_call');

    const { filteredIndices, currentIndex } = useReplayStore.getState();
    expect(filteredIndices).toEqual([0, 2]);
    expect(currentIndex).toBe(0);
  });
});

describe('appendLiveEvent', () => {
  it('追加事件 + 跳转到最新', () => {
    useReplayStore.getState().setEvents([mockEvent()]);
    const newEvent = mockEvent('decision');

    useReplayStore.getState().appendLiveEvent(newEvent);

    const { events, filteredIndices, currentIndex } = useReplayStore.getState();
    expect(events).toHaveLength(2);
    expect(events[1]).toEqual(newEvent);
    expect(filteredIndices).toEqual([0, 1]);
    expect(currentIndex).toBe(1);
  });
});

describe('speed localStorage 持久化', () => {
  it('setSpeed 后 localStorage 有值', () => {
    useReplayStore.getState().setSpeed(4);

    expect(useReplayStore.getState().speed).toBe(4);
    expect(localStorage.getItem('replay_speed')).toBe('4');
  });
});

describe('reset', () => {
  it('清空所有状态', () => {
    useReplayStore.getState().setEvents([mockEvent(), mockEvent()]);
    useReplayStore.getState().setSessionId('sess-123');
    useReplayStore.getState().setError('boom');

    useReplayStore.getState().reset();

    const state = useReplayStore.getState();
    expect(state.mode).toBe('loading');
    expect(state.events).toEqual([]);
    expect(state.filteredIndices).toEqual([]);
    expect(state.currentIndex).toBe(0);
    expect(state.filterType).toBeNull();
    expect(state.errorMessage).toBe('');
    expect(state.sessionId).toBe('');
  });
});
