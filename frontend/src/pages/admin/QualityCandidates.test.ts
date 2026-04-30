import { describe, expect, it } from 'vitest';
import type { QualityCandidateRecord } from '../../types/api';
import { buildStatusUpdate, derivePromotedCaseID, summarizeQualityCandidates } from './QualityCandidates.logic';

function candidate(overrides: Partial<QualityCandidateRecord> = {}): QualityCandidateRecord {
  return {
    id: 'candidate_DEAD-BEEF',
    status: 'new',
    route: 'web',
    session_id: 'session-1',
    replay_ref: 'session-1:step-1',
    input: '执行 rm -rf ./tmp-cache',
    case: {
      id: 'candidate_DEAD-BEEF',
      name: '失败回归候选',
      route: 'web',
      input: '执行 rm -rf ./tmp-cache',
      expected_status: 'fail',
      failure_type: 'permission',
      risk: 'dangerous',
      required: false,
    },
    failure_type: 'permission',
    risk: 'dangerous',
    fingerprint: 'sha256:deadbeef',
    source_event: {},
    created_at: '2026-04-29T10:00:00Z',
    updated_at: '2026-04-29T10:00:00Z',
    ...overrides,
  };
}

describe('QualityCandidates helpers', () => {
  it('summarizes risk, approved/promoted, and delegation candidates', () => {
    const items = [
      candidate(),
      candidate({ id: 'approved', status: 'approved', risk: 'safe', route: 'im' }),
      candidate({ id: 'promoted', status: 'promoted', risk: 'safe', route: 'acp' }),
      candidate({
        id: 'delegation',
        risk: 'safe',
        case: { ...candidate().case, expected_status: 'needs_user' },
      }),
    ];

    expect(summarizeQualityCandidates(items)).toEqual({
      dangerous: 1,
      approved: 2,
      delegation: 2,
    });
  });

  it('derives a stable promoted_case_id without requiring manual fixture edits', () => {
    expect(derivePromotedCaseID(candidate())).toBe('aq_candidate_dead_beef');
    expect(derivePromotedCaseID(candidate({ promoted_case_id: ' aq08_tool_choice ' }))).toBe('aq08_tool_choice');
  });

  it('builds promote request with generated case id and trimmed review note', () => {
    expect(buildStatusUpdate(candidate(), 'promote', ' 已脱敏，可复现 ')).toEqual({
      status: 'promoted',
      review_note: '已脱敏，可复现',
      promoted_case_id: 'aq_candidate_dead_beef',
    });
  });
});
