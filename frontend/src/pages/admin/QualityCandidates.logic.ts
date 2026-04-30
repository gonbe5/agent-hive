import type { QualityCandidateRecord, QualityCandidateStatus, QualityCandidateUpdateRequest } from '../../types/api';

export type CandidateSummary = { dangerous: number; approved: number; delegation: number };
export type CandidateAction = 'reviewing' | 'approve' | 'reject' | 'reset' | 'promote';

export function summarizeQualityCandidates(items: QualityCandidateRecord[]): CandidateSummary {
  return items.reduce((acc, item) => {
    if (item.risk === 'dangerous') acc.dangerous++;
    if (item.status === 'approved' || item.status === 'promoted') acc.approved++;
    const event = item.source_event ?? {};
    if (item.route === 'acp' || event.delegation || item.case?.expected_status === 'needs_user') acc.delegation++;
    return acc;
  }, { dangerous: 0, approved: 0, delegation: 0 });
}

export function derivePromotedCaseID(item: QualityCandidateRecord): string {
  if (item.promoted_case_id?.trim()) return item.promoted_case_id.trim();
  const base = item.case?.id || item.id || 'quality_candidate';
  return sanitizeCaseID(base.replace(/^candidate_/, 'aq_candidate_'));
}

export function buildStatusUpdate(
  item: QualityCandidateRecord,
  action: CandidateAction,
  reviewNote: string,
): QualityCandidateUpdateRequest {
  const note = reviewNote.trim();
  const body: QualityCandidateUpdateRequest = {
    status: actionToStatus(action),
  };
  if (note) body.review_note = note;
  if (action === 'promote') {
    body.promoted_case_id = derivePromotedCaseID(item);
  }
  return body;
}

function actionToStatus(action: CandidateAction): QualityCandidateStatus {
  if (action === 'approve') return 'approved';
  if (action === 'reject') return 'rejected';
  if (action === 'reset') return 'new';
  if (action === 'promote') return 'promoted';
  return action;
}

function sanitizeCaseID(value: string): string {
  const normalized = value.trim().toLowerCase().replace(/[^a-z0-9_]+/g, '_').replace(/^_+|_+$/g, '');
  return normalized || 'quality_candidate';
}
