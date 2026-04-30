import { useEffect, useState, useCallback, type ReactNode } from 'react';
import { AlertTriangle, CheckCircle2, Download, GitBranch, RefreshCcw, ShieldCheck, Sparkles } from 'lucide-react';
import { useNodeClient } from '../../hooks/useNodeClient';
import { useToastStore } from '../../store/toast';
import type { QualityCandidateRecord, QualityCandidateStatus } from '../../types/api';
import {
  buildStatusUpdate,
  derivePromotedCaseID,
  summarizeQualityCandidates,
  type CandidateAction,
} from './QualityCandidates.logic';

const statusOptions: Array<{ value: QualityCandidateStatus | ''; label: string }> = [
  { value: '', label: '全部状态' },
  { value: 'new', label: '待审核' },
  { value: 'reviewing', label: '审核中' },
  { value: 'approved', label: '已通过' },
  { value: 'rejected', label: '已拒绝' },
  { value: 'promoted', label: '已晋升' },
];
const PAGE_SIZE = 50;
const buttonClass = 'px-3 py-2 rounded-lg border border-[var(--border-color)] text-sm text-[var(--text-primary)] hover:bg-[var(--bg-secondary)]';
const dangerButtonClass = 'px-3 py-2 rounded-lg border border-red-200 text-sm text-red-700 hover:bg-red-50';
const successButtonClass = 'px-3 py-2 rounded-lg border border-emerald-200 text-sm text-emerald-700 hover:bg-emerald-50';

export function QualityCandidates() {
  const client = useNodeClient();
  const addToast = useToastStore((s) => s.addToast);
  const [items, setItems] = useState<QualityCandidateRecord[]>([]);
  const [total, setTotal] = useState(0);
  const [status, setStatus] = useState<QualityCandidateStatus | ''>('');
  const [route, setRoute] = useState('');
  const [loading, setLoading] = useState(true);
  const [selected, setSelected] = useState<QualityCandidateRecord | null>(null);
  const [reviewNote, setReviewNote] = useState('');
  const [goldenPreview, setGoldenPreview] = useState<QualityCandidateRecord['golden_case'] | null>(null);
  const [busyAction, setBusyAction] = useState<CandidateAction | 'export' | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await client.adminListQualityCandidates({ status, route, page: 1, size: PAGE_SIZE });
      setItems(res.candidates ?? []);
      setTotal(res.total ?? 0);
      setSelected((current) => {
        if (!current) return null;
        return res.candidates.find((item) => item.id === current.id) ?? current;
      });
    } catch (e: unknown) {
      addToast('error', e instanceof Error ? e.message : '加载质量候选用例失败');
    } finally {
      setLoading(false);
    }
  }, [client, status, route, addToast]);

  useEffect(() => { load(); }, [load]);

  const updateStatus = async (item: QualityCandidateRecord, action: CandidateAction) => {
    const body = buildStatusUpdate(item, action, reviewNote);
    setBusyAction(action);
    try {
      const updated = await client.adminUpdateQualityCandidate(item.id, body);
      addToast('success', statusActionSuccess(action, updated));
      setSelected(updated);
      if (updated.golden_case) setGoldenPreview(updated.golden_case);
      await load();
    } catch (e: unknown) {
      addToast('error', e instanceof Error ? e.message : '更新候选状态失败');
    } finally {
      setBusyAction(null);
    }
  };

  const exportGoldenCase = async (item: QualityCandidateRecord) => {
    setBusyAction('export');
    try {
      const exported = await client.adminExportQualityCandidate(item.id);
      setGoldenPreview(exported ?? null);
      addToast('success', '已生成 golden_case 预览；未写入正式 fixture');
    } catch (e: unknown) {
      addToast('error', e instanceof Error ? e.message : '导出 golden_case 失败');
    } finally {
      setBusyAction(null);
    }
  };

  const counts = summarize(items);

  return (
    <div className="p-6 max-w-7xl mx-auto">
      <div className="mb-6 flex items-start justify-between gap-4">
        <div>
          <h1 className="text-xl font-semibold text-[var(--text-primary)] font-display">质量候选用例</h1>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">
            失败质量事件进入数据库候选池；只有人工审核后，才允许晋升为正式 golden case。
          </p>
        </div>
        <button
          onClick={load}
          className="inline-flex items-center gap-2 px-3 py-2 rounded-lg border border-[var(--border-color)] text-sm text-[var(--text-primary)] hover:bg-[var(--bg-secondary)]"
        >
          <RefreshCcw size={14} />
          刷新
        </button>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3 mb-5">
        <StatCard title="当前列表" value={String(total)} icon={<AlertTriangle size={17} />} />
        <StatCard title="危险边界" value={String(counts.dangerous)} icon={<ShieldCheck size={17} />} tone="danger" />
        <StatCard title="已通过" value={String(counts.approved)} icon={<CheckCircle2 size={17} />} tone="success" />
        <StatCard title="委派/ACP" value={String(counts.delegation)} icon={<GitBranch size={17} />} />
      </div>

      <div className="mb-4 flex flex-wrap items-center gap-3">
        <select
          value={status}
          onChange={(e) => setStatus(e.target.value as QualityCandidateStatus | '')}
          className="px-3 py-2 rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] text-sm text-[var(--text-primary)]"
        >
          {statusOptions.map((option) => (
            <option key={option.value || 'all'} value={option.value}>{option.label}</option>
          ))}
        </select>
        <input
          value={route}
          onChange={(e) => setRoute(e.target.value)}
          placeholder="route: web / im / acp"
          className="px-3 py-2 rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] text-sm text-[var(--text-primary)] placeholder:text-[var(--text-secondary)]"
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_420px] gap-5">
        <div className="rounded-xl border border-[var(--border-color)] overflow-hidden bg-[var(--bg-primary)]">
          <table className="w-full text-sm">
            <thead className="bg-[var(--bg-secondary)]">
              <tr>
                <th className="px-4 py-3 text-left font-medium text-[var(--text-secondary)]">状态</th>
                <th className="px-4 py-3 text-left font-medium text-[var(--text-secondary)]">失败类型</th>
                <th className="px-4 py-3 text-left font-medium text-[var(--text-secondary)]">风险</th>
                <th className="px-4 py-3 text-left font-medium text-[var(--text-secondary)]">Route</th>
                <th className="px-4 py-3 text-left font-medium text-[var(--text-secondary)]">输入</th>
                <th className="px-4 py-3 text-left font-medium text-[var(--text-secondary)]">创建时间</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[var(--border-color)]">
              {loading ? (
                <tr>
                  <td colSpan={6} className="px-4 py-10 text-center text-[var(--text-secondary)] animate-pulse">加载中...</td>
                </tr>
              ) : items.length === 0 ? (
                <tr>
                  <td colSpan={6} className="px-4 py-10 text-center text-[var(--text-secondary)]">暂无候选用例</td>
                </tr>
              ) : items.map((item) => (
                <tr
                  key={item.id}
                  onClick={() => {
                    setSelected(item);
                    setReviewNote(item.review_note ?? '');
                    setGoldenPreview(item.golden_case ?? null);
                  }}
                  className={`cursor-pointer hover:bg-[var(--bg-secondary)] ${selected?.id === item.id ? 'bg-[var(--bg-secondary)]' : ''}`}
                >
                  <td className="px-4 py-3"><StatusBadge status={item.status} /></td>
                  <td className="px-4 py-3 text-[var(--text-primary)]">{item.failure_type || '-'}</td>
                  <td className="px-4 py-3">
                    <span className={item.risk === 'dangerous' ? 'text-red-600 font-medium' : 'text-[var(--text-secondary)]'}>
                      {item.risk || 'safe'}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-[var(--text-secondary)]">{item.route || '-'}</td>
                  <td className="px-4 py-3 text-[var(--text-primary)] max-w-xs truncate">{item.input}</td>
                  <td className="px-4 py-3 text-[var(--text-secondary)]">{formatTime(item.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <aside className="rounded-xl border border-[var(--border-color)] bg-[var(--bg-primary)] p-4 min-h-[420px]">
          {selected ? (
            <CandidateDetail
              item={selected}
              reviewNote={reviewNote}
              setReviewNote={setReviewNote}
              updateStatus={updateStatus}
              exportGoldenCase={exportGoldenCase}
              goldenPreview={goldenPreview}
              busyAction={busyAction}
            />
          ) : (
            <div className="h-full flex items-center justify-center text-center text-[var(--text-secondary)] text-sm">
              选择一条候选查看 case/source event，并执行审核动作。
            </div>
          )}
        </aside>
      </div>
    </div>
  );
}

function CandidateDetail(props: {
  item: QualityCandidateRecord;
  reviewNote: string;
  setReviewNote: (v: string) => void;
  updateStatus: (item: QualityCandidateRecord, action: CandidateAction) => Promise<void>;
  exportGoldenCase: (item: QualityCandidateRecord) => Promise<void>;
  goldenPreview: QualityCandidateRecord['golden_case'] | null;
  busyAction: CandidateAction | 'export' | null;
}) {
  const { item, reviewNote, setReviewNote, updateStatus, exportGoldenCase, goldenPreview, busyAction } = props;
  const canPromote = item.status === 'approved';
  const canExport = item.status === 'promoted' || Boolean(item.golden_case);
  return (
    <div className="space-y-4">
      <div>
        <div className="flex items-center justify-between gap-3 mb-2">
          <h2 className="text-sm font-semibold text-[var(--text-primary)] break-all">{item.id}</h2>
          <StatusBadge status={item.status} />
        </div>
        <p className="text-xs text-[var(--text-secondary)] break-all">fingerprint: {item.fingerprint}</p>
        <p className="text-xs text-[var(--text-secondary)] break-all">replay: {item.replay_ref || '-'}</p>
        <p className="text-xs text-[var(--text-secondary)] break-all">promoted_case_id: {derivePromotedCaseID(item)}</p>
      </div>

      <textarea
        value={reviewNote}
        onChange={(e) => setReviewNote(e.target.value)}
        placeholder="审核备注，例如：可复现，晋升前需要脱敏路径"
        rows={3}
        className="w-full px-3 py-2 rounded-lg border border-[var(--border-color)] bg-[var(--bg-primary)] text-sm text-[var(--text-primary)] placeholder:text-[var(--text-secondary)]"
      />

      <div className="grid grid-cols-2 gap-2">
        <button disabled={busyAction != null} onClick={() => updateStatus(item, 'reviewing')} className={buttonClass}>标记审核中</button>
        <button disabled={busyAction != null} onClick={() => updateStatus(item, 'approve')} className={successButtonClass}>通过</button>
        <button disabled={busyAction != null} onClick={() => updateStatus(item, 'reject')} className={dangerButtonClass}>拒绝</button>
        <button disabled={busyAction != null} onClick={() => updateStatus(item, 'reset')} className={buttonClass}>退回待审核</button>
      </div>

      <div className="space-y-2">
        <button
          disabled={!canPromote || busyAction != null}
          onClick={() => updateStatus(item, 'promote')}
          className={`w-full inline-flex items-center justify-center gap-2 ${successButtonClass} disabled:opacity-50 disabled:cursor-not-allowed`}
          title={canPromote ? '晋升后只返回 golden_case 预览，不自动写正式 fixture' : '必须先 approve 才能 promote'}
        >
          <Sparkles size={14} />
          Promote 并预览 golden_case
        </button>
        <button
          disabled={!canExport || busyAction != null}
          onClick={() => exportGoldenCase(item)}
          className={`w-full inline-flex items-center justify-center gap-2 ${buttonClass} disabled:opacity-50 disabled:cursor-not-allowed`}
        >
          <Download size={14} />
          Export golden_case 预览
        </button>
        <p className="text-[11px] leading-5 text-[var(--text-secondary)]">
          晋升仅调用 API 并展示返回 golden_case；前端不会写入正式 fixture。
        </p>
      </div>

      <OptimizationSuggestions item={item} />
      {goldenPreview && <JsonBlock title="golden_case_preview" value={goldenPreview} />}
      <JsonBlock title="case_json" value={item.case} />
      <JsonBlock title="source_event" value={item.source_event} />
    </div>
  );
}

function OptimizationSuggestions({ item }: { item: QualityCandidateRecord }) {
  const suggestions = item.optimization_suggestions ?? [];
  if (suggestions.length === 0) {
    return (
      <div className="rounded-lg border border-[var(--border-color)] bg-[var(--bg-secondary)] p-3 text-xs text-[var(--text-secondary)]">
        暂无自动优化草稿；这条候选只进入回归池，不建议修改 prompt / skill / tool。
      </div>
    );
  }
  return (
    <div className="space-y-2">
      <div className="text-xs font-semibold text-[var(--text-secondary)]">只读优化建议</div>
      {suggestions.map((s, idx) => (
        <div key={`${s.kind}-${idx}`} className="rounded-lg border border-amber-200 bg-amber-50 p-3">
          <div className="flex items-center justify-between gap-2">
            <span className="text-xs font-semibold text-amber-900">{suggestionKindLabel(s.kind)}</span>
            <span className="text-[11px] text-amber-700">{s.target || 'review only'}</span>
          </div>
          <div className="mt-1 text-sm font-medium text-amber-950">{s.title}</div>
          <p className="mt-1 text-xs text-amber-800">{s.rationale}</p>
          <pre className="mt-2 max-h-44 overflow-auto rounded-md bg-white/70 p-2 text-xs text-amber-950 whitespace-pre-wrap">
            {s.proposed}
          </pre>
        </div>
      ))}
    </div>
  );
}

function suggestionKindLabel(kind: string) {
  if (kind === 'prompt_diff_suggestion') return 'Prompt diff 草稿';
  if (kind === 'tool_description_suggestion') return 'Tool 描述草稿';
  if (kind === 'skill_draft') return 'Skill 草稿';
  return kind;
}

function StatCard({ title, value, icon, tone }: { title: string; value: string; icon: ReactNode; tone?: 'danger' | 'success' }) {
  const color = tone === 'danger' ? 'text-red-600' : tone === 'success' ? 'text-emerald-600' : 'text-[var(--accent)]';
  return (
    <div className="rounded-xl border border-[var(--border-color)] bg-[var(--bg-primary)] p-4">
      <div className={`flex items-center gap-2 text-sm font-medium ${color}`}>{icon}{title}</div>
      <div className="mt-2 text-2xl font-semibold text-[var(--text-primary)]">{value}</div>
    </div>
  );
}

function StatusBadge({ status }: { status: QualityCandidateStatus }) {
  const cls = status === 'approved' || status === 'promoted'
    ? 'bg-emerald-50 text-emerald-700 border-emerald-200'
    : status === 'rejected'
      ? 'bg-red-50 text-red-700 border-red-200'
      : 'bg-amber-50 text-amber-700 border-amber-200';
  return <span className={`inline-flex px-2 py-1 rounded-full text-xs border ${cls}`}>{status}</span>;
}

function JsonBlock({ title, value }: { title: string; value: unknown }) {
  return (
    <div>
      <div className="mb-1 text-xs font-semibold text-[var(--text-secondary)]">{title}</div>
      <pre className="max-h-56 overflow-auto rounded-lg bg-[var(--bg-secondary)] p-3 text-xs text-[var(--text-primary)] whitespace-pre-wrap">
        {JSON.stringify(value, null, 2)}
      </pre>
    </div>
  );
}

const summarize = summarizeQualityCandidates;

function statusActionSuccess(action: CandidateAction, updated: QualityCandidateRecord): string {
  if (action === 'promote') {
    return updated.golden_case ? '已晋升并返回 golden_case 预览' : '已晋升；后端未返回 golden_case 预览';
  }
  return `候选状态已更新为 ${updated.status}`;
}

function formatTime(value: string) {
  if (!value) return '-';
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return d.toLocaleString();
}
