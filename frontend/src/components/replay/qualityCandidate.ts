import type { Message, QualityCandidateCreateRequest } from '../../types/api';
import type { JournalEvent, QualityEventView } from '../../types/journal';

export function buildQualityCandidateRequest(
  sessionID: string,
  event: JournalEvent | null,
  eventIndex: number,
  messages: Message[],
): QualityCandidateCreateRequest | null {
  if (!sessionID || !event?.quality_event || !isCandidateWorthy(event.quality_event)) {
    return null;
  }

  const replayRef = `${sessionID}:step-${eventIndex}`;
  const input = findNearestUserInput(messages, event.timestamp);
  if (!input) {
    return null;
  }

  return {
    session_id: sessionID,
    replay_ref: replayRef,
    event_index: eventIndex,
    input,
    quality_event: {
      ...event.quality_event,
      replay_ref: replayRef,
    },
  };
}

function isCandidateWorthy(ev: QualityEventView): boolean {
  if (ev.final_status === 'fail' || ev.final_status === 'blocked' || ev.final_status === 'needs_user') {
    return true;
  }
  return Boolean(ev.failure_type && ev.failure_type !== 'none' && ev.final_status !== 'pass');
}

function findNearestUserInput(messages: Message[], eventTimestamp: string): string {
  const eventMs = Date.parse(eventTimestamp);
  const candidates = [...messages].filter((msg) => {
    if (msg.role !== 'user' || !msg.content.trim()) return false;
    const msgMs = Date.parse(msg.timestamp ?? '');
    return Number.isNaN(eventMs) || Number.isNaN(msgMs) || msgMs <= eventMs;
  });
  const last = candidates.at(-1);
  return last?.content.trim() ?? '';
}
