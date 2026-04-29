import { create } from 'zustand';
import type { JournalEvent, JournalEventType } from '../types/journal';

const SPEED_KEY = 'replay_speed';

export type ReplayMode = 'loading' | 'empty' | 'error' | 'ready' | 'playing' | 'paused' | 'live';

interface ReplayState {
  mode: ReplayMode;
  events: JournalEvent[];
  filteredIndices: number[];
  currentIndex: number;
  speed: number;
  filterType: JournalEventType | null;
  errorMessage: string;
  sessionId: string;

  setEvents: (events: JournalEvent[]) => void;
  setMode: (mode: ReplayMode) => void;
  setError: (msg: string) => void;
  setSessionId: (id: string) => void;
  setSpeed: (speed: number) => void;
  setFilterType: (type: JournalEventType | null) => void;
  setCurrentIndex: (index: number) => void;
  play: () => void;
  pause: () => void;
  stepForward: () => void;
  stepBackward: () => void;
  appendLiveEvent: (event: JournalEvent) => void;
  reset: () => void;
}

function buildFilteredIndices(events: JournalEvent[], filterType: JournalEventType | null): number[] {
  if (!filterType) return events.map((_, i) => i);
  return events.reduce<number[]>((acc, e, i) => {
    if (e.type === filterType) acc.push(i);
    return acc;
  }, []);
}

function loadSpeed(): number {
  try {
    const v = localStorage.getItem(SPEED_KEY);
    if (v) {
      const n = Number(v);
      if ([1, 2, 4].includes(n)) return n;
    }
  } catch { /* ignore */ }
  return 1;
}

export const useReplayStore = create<ReplayState>((set, get) => ({
  mode: 'loading',
  events: [],
  filteredIndices: [],
  currentIndex: 0,
  speed: loadSpeed(),
  filterType: null,
  errorMessage: '',
  sessionId: '',

  setEvents: (events) => {
    const { filterType } = get();
    set({
      events,
      filteredIndices: buildFilteredIndices(events, filterType),
      currentIndex: 0,
      mode: events.length === 0 ? 'empty' : 'ready',
    });
  },

  setMode: (mode) => set({ mode }),

  setError: (msg) => set({ mode: 'error', errorMessage: msg }),

  setSessionId: (id) => set({ sessionId: id }),

  setSpeed: (speed) => {
    try { localStorage.setItem(SPEED_KEY, String(speed)); } catch { /* ignore */ }
    set({ speed });
  },

  setFilterType: (type) => {
    const { events } = get();
    set({
      filterType: type,
      filteredIndices: buildFilteredIndices(events, type),
      currentIndex: 0,
    });
  },

  setCurrentIndex: (index) => set({ currentIndex: index }),

  play: () => {
    const { mode, events } = get();
    if (events.length === 0) return;
    if (mode === 'ready' || mode === 'paused') {
      set({ mode: 'playing' });
    }
  },

  pause: () => {
    if (get().mode === 'playing') {
      set({ mode: 'paused' });
    }
  },

  stepForward: () => {
    const { currentIndex, filteredIndices, mode } = get();
    if (currentIndex < filteredIndices.length - 1) {
      set({ currentIndex: currentIndex + 1, mode: mode === 'playing' ? 'playing' : 'paused' });
    }
  },

  stepBackward: () => {
    const { currentIndex, mode } = get();
    if (currentIndex > 0) {
      set({ currentIndex: currentIndex - 1, mode: mode === 'playing' ? 'playing' : 'paused' });
    }
  },

  appendLiveEvent: (event) => {
    const { events, filterType } = get();
    const newEvents = [...events, event];
    set({
      events: newEvents,
      filteredIndices: buildFilteredIndices(newEvents, filterType),
      currentIndex: buildFilteredIndices(newEvents, filterType).length - 1,
    });
  },

  reset: () => set({
    mode: 'loading',
    events: [],
    filteredIndices: [],
    currentIndex: 0,
    filterType: null,
    errorMessage: '',
    sessionId: '',
  }),
}));
