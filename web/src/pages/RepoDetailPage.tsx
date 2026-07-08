import { useEffect, useState, useCallback } from 'react'
import { useNavigate, useSearchParams, Link, useLocation } from 'react-router-dom'
import { fetchRepoJobs, fetchIsolatedJobs, upsertJobMeta, deleteJobRuns } from '../api'
import type { JobSummaryRow } from '../types'
import { formatFromUSD, useCurrencyPreference } from '../currency'

function timeAgo(iso: string) {
  const secs = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (secs < 60) return `${secs}s ago`
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`
  return `${Math.floor(secs / 86400)}d ago`
}

// ── Snooze modal ──────────────────────────────────────────────────────────────
interface SnoozeModalProps { row: JobSummaryRow; onClose: () => void; onSaved: () => void }
function SnoozeModal({ row, onClose, onSaved }: SnoozeModalProps) {
  const [days, setDays]     = useState(7)
  const [reason, setReason] = useState('')
  const [saving, setSaving] = useState(false)
  const [err, setErr]       = useState('')
  const isCurrentlySnoozed = row.snoozed_until != null && new Date(row.snoozed_until) > new Date()

  async function handleSnooze() {
    setSaving(true); setErr('')
    try {
      await upsertJobMeta({ job_id: row.job_id, repository: row.repository,
        snoozed_until: new Date(Date.now() + days * 86400_000).toISOString(), snooze_reason: reason })
      onSaved(); onClose()
    } catch (e) { setErr(String(e)) } finally { setSaving(false) }
  }
  async function handleUnsnooze() {
    setSaving(true); setErr('')
    try {
      await upsertJobMeta({ job_id: row.job_id, repository: row.repository, snoozed_until: null, snooze_reason: '' })
      onSaved(); onClose()
    } catch (e) { setErr(String(e)) } finally { setSaving(false) }
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-[200] px-4" onClick={onClose}>
      <div className="bg-paper border border-[var(--border)] shadow-rr-lg rounded-xl p-7 w-full max-w-sm" onClick={e => e.stopPropagation()}>
        <h2 className="font-serif text-[1.1rem] font-bold text-[var(--text)] mb-4">Snooze — {row.job_id}</h2>
        {isCurrentlySnoozed ? (
          <>
            <p className="text-sm text-[var(--text-mid)] mb-5">
              Snoozed until <strong>{new Date(row.snoozed_until!).toLocaleDateString()}</strong>
              {row.snooze_reason && <> — {row.snooze_reason}</>}.
            </p>
            <div className="flex gap-2 justify-end">
              <button onClick={handleUnsnooze} disabled={saving}
                className="btn-rr !bg-red !text-cream text-sm py-2 px-4 disabled:opacity-50">Remove snooze</button>
              <button onClick={onClose} className="px-4 py-2 bg-[var(--cream-alt)] border border-[var(--border)] text-[var(--text-mid)] text-sm font-sans cursor-pointer">Cancel</button>
            </div>
          </>
        ) : (
          <>
            <label className="block font-deco text-[11px] tracking-[1.5px] text-[var(--text-mid)] uppercase mb-1.5">Days to snooze</label>
            <input type="number" min={1} max={365} value={days} onChange={e => setDays(Number(e.target.value))}
              className="rr-input mb-4" />
            <label className="block font-deco text-[11px] tracking-[1.5px] text-[var(--text-mid)] uppercase mb-1.5">Reason (optional)</label>
            <input type="text" placeholder="e.g. investigating new machine type" value={reason} onChange={e => setReason(e.target.value)}
              className="rr-input mb-4" />
            {err && <p className="text-red text-sm mb-3">{err}</p>}
            <div className="flex gap-2 justify-end">
              <button onClick={handleSnooze} disabled={saving}
                className="btn-rr text-sm py-2 px-4 disabled:opacity-50">{saving ? 'Saving…' : `Snooze ${days}d`}</button>
              <button onClick={onClose} className="px-4 py-2 bg-[var(--cream-alt)] border border-[var(--border)] text-[var(--text-mid)] text-sm font-sans cursor-pointer">Cancel</button>
            </div>
          </>
        )}
      </div>
    </div>
  )
}

// ── StatusBadge ───────────────────────────────────────────────────────────────
function StatusBadge({ row, snoozed }: { row: JobSummaryRow; snoozed: boolean }) {
  if (row.archived) return <span className="badge" style={{ background: '#f3f4f6', color: '#6b7280', borderColor: '#6b7280' }}>archived</span>
  if (snoozed) return <span className="badge" style={{ background: '#e0e7ff', color: '#3730a3', borderColor: '#3730a3' }} title={row.snooze_reason}>snoozed</span>
  if (row.stale) return <span className="badge" style={{ background: '#fef3c7', color: '#92400e', borderColor: '#92400e' }}>stale</span>
  return <span className="badge badge-right-sized">active</span>
}

function rowBenefit(row: JobSummaryRow): { label: string; tone: 'saving' | 'benefit' | 'neutral' } {
  const rec = row.latest_recommendations?.[0]
  if (row.monthly_savings_usd > 0 || (rec?.cost_delta_percent ?? 0) < -0.5) {
    return { label: 'Cost reduction', tone: 'saving' }
  }
  if (rec?.tier === 'more-headroom' || (rec?.cost_delta_percent ?? 0) > 0.5) {
    return { label: 'Performance / stability', tone: 'benefit' }
  }
  return { label: 'Right-sized fit', tone: 'neutral' }
}

function benefitStyle(tone: 'saving' | 'benefit' | 'neutral'): React.CSSProperties {
  switch (tone) {
    case 'saving':
      return { color: '#2E7D32', borderColor: '#2E7D32', background: 'rgba(46,125,50,.08)' }
    case 'benefit':
      return { color: '#1B3361', borderColor: '#1B3361', background: 'rgba(27,51,97,.08)' }
    default:
      return { color: '#9A7B5A', borderColor: '#9A7B5A', background: 'rgba(154,123,90,.08)' }
  }
}

function spotStyle(risk: string): React.CSSProperties {
  switch (risk) {
    case 'low':
      return { color: '#2E7D32', borderColor: '#2E7D32', background: 'rgba(46,125,50,.08)' }
    case 'high':
      return { color: '#C23B22', borderColor: '#C23B22', background: 'rgba(194,59,34,.08)' }
    default:
      return { color: '#9A7B5A', borderColor: '#9A7B5A', background: 'rgba(154,123,90,.08)' }
  }
}

// ── Main page ─────────────────────────────────────────────────────────────────
export default function RepoDetailPage() {
  const { currency } = useCurrencyPreference()
  const navigate = useNavigate()
  const location = useLocation()
  const [params] = useSearchParams()
  const repo     = params.get('repo') ?? ''
  const isolated = params.get('isolated') === 'true'

  const [jobs, setJobs]               = useState<JobSummaryRow[]>([])
  const [showArchived, setShowArchived] = useState(false)
  const [loading, setLoading]         = useState(true)
  const [error, setError]             = useState('')
  const [snoozeTarget, setSnoozeTarget] = useState<JobSummaryRow | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<JobSummaryRow | null>(null)
  const [deleting, setDeleting]       = useState(false)

  const load = useCallback(() => {
    setLoading(true)
    const fetcher = isolated ? fetchIsolatedJobs(showArchived) : fetchRepoJobs(repo, showArchived)
    fetcher.then(setJobs).catch(e => setError(String(e))).finally(() => setLoading(false))
  }, [repo, isolated, showArchived])

  useEffect(() => { load() }, [load])

  const handleArchive = async (row: JobSummaryRow) => {
    try { await upsertJobMeta({ job_id: row.job_id, repository: row.repository, archived: !row.archived }); load() }
    catch (e) { alert(String(e)) }
  }
  const handleDelete = async () => {
    if (!confirmDelete) return
    setDeleting(true)
    try { await deleteJobRuns(confirmDelete.job_id, confirmDelete.repository); setConfirmDelete(null); load() }
    catch (e) { alert(String(e)) }
    finally { setDeleting(false) }
  }

  const title = isolated ? 'Isolated jobs' : repo
  const backTo = `${location.pathname}${location.search}`

  return (
    <div className="fadein max-w-[1100px]">
      {/* Breadcrumb */}
      <nav className="flex items-center gap-2 text-sm text-[var(--text-light)] mb-3 font-sans">
        <Link to="/app/repos" className="text-navy hover:underline hover:text-[var(--red)] no-underline">Repositories</Link>
        <span className="text-[var(--border)]">/</span>
        <span className="text-[var(--text-mid)]">{title}</span>
      </nav>

      <button
        type="button"
        onClick={() => navigate('/app/repos')}
        className="mb-4 inline-flex items-center gap-2 text-sm font-deco tracking-widest text-[var(--text-mid)] hover:text-[var(--text)] transition-colors"
      >
        <span aria-hidden="true">←</span>
        Back to repos
      </button>

      <div className="flex flex-wrap items-baseline justify-between gap-3 mb-6">
        <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] tracking-tight break-all">{title}</h1>
        <label className="flex items-center gap-2 text-sm text-[var(--text-light)] cursor-pointer font-sans">
          <input type="checkbox" checked={showArchived} onChange={e => setShowArchived(e.target.checked)} />
          Show archived
        </label>
      </div>

      {loading && <div className="empty text-base">Loading jobs…</div>}
      {error   && <div className="empty text-base text-red">{error}</div>}

      {!loading && !error && jobs.length === 0 && (
        <div className="empty text-base">No jobs found.</div>
      )}

      {!loading && jobs.length > 0 && (
        <>
        <div className="sm:hidden space-y-3 mb-4">
          {jobs.map((j) => {
            const snoozed = j.snoozed_until != null && new Date(j.snoozed_until) > new Date()
            return (
              <button
                key={`${j.job_id}-${j.repository}-mobile`}
                className={`w-full text-left bg-paper border border-[var(--border)] rounded-lg px-4 py-3 shadow-rr ${j.archived ? 'opacity-50' : ''}`}
                onClick={() => navigate(`/app/jobs/group/${encodeURIComponent(j.job_id)}`, { state: { backTo, backLabel: 'Back to repos' } })}
              >
                <div className="flex items-start justify-between gap-3 mb-2">
                  <div>
                    <div className="font-mono text-[13px] text-[var(--text)] break-all">{j.job_id}</div>
                    <div className="text-xs text-[var(--text-light)] mt-1">{timeAgo(j.last_seen)}</div>
                  </div>
                  <StatusBadge row={j} snoozed={snoozed} />
                </div>
                <div className="grid grid-cols-2 gap-2 text-xs text-[var(--text-mid)]">
                  <div><span className="text-[var(--text-light)]">Runs</span><div>{j.run_count}</div></div>
                  <div><span className="text-[var(--text-light)]">Savings/mo</span><div>{j.monthly_savings_usd > 0 ? formatFromUSD(j.monthly_savings_usd, currency, { minimumFractionDigits: 0, maximumFractionDigits: 0 }) : '—'}</div></div>
                  <div><span className="text-[var(--text-light)]">Benefit</span><div><span className="badge" style={benefitStyle(rowBenefit(j).tone)}>{rowBenefit(j).label}</span></div></div>
                  <div><span className="text-[var(--text-light)]">Spot</span><div>{j.latest_recommendations?.[0]?.spot_risk ? <span className="badge" style={spotStyle(j.latest_recommendations[0].spot_risk!)}>{j.latest_recommendations[0].spot_risk}</span> : '—'}</div></div>
                </div>
                <div className="flex gap-2 mt-3 justify-end">
                  {!j.archived && <span className="text-xs text-[var(--text-light)]">Snooze</span>}
                  <span className="text-xs text-[var(--text-light)]">{j.archived ? 'Unarchive' : 'Archive'}</span>
                  <span className="text-xs text-red">Delete</span>
                </div>
              </button>
            )
          })}
        </div>

        <div className="rr-card !p-0 hidden sm:block">
          <div className="table-wrap">
            <table className="rr-table min-w-[860px] table-fixed">
              <colgroup>
                <col style={{ width: '28%' }} />
                <col style={{ width: '12%' }} />
                <col style={{ width: '7%' }} />
                <col style={{ width: '12%' }} />
                <col style={{ width: '12%' }} />
                <col style={{ width: '12%' }} />
                <col style={{ width: '7%' }} />
                <col style={{ width: '10%' }} />
              </colgroup>
              <thead>
                <tr>
                  <th>Job ID</th>
                  <th className="hidden sm:table-cell">Last seen</th>
                  <th className="hidden sm:table-cell">Runs</th>
                  <th>Status</th>
                  <th className="hidden md:table-cell">Savings/mo</th>
                  <th className="hidden md:table-cell">Benefit</th>
                  <th className="hidden md:table-cell">Spot</th>
                  <th className="text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {jobs.map((j) => {
                  const snoozed = j.snoozed_until != null && new Date(j.snoozed_until) > new Date()
                  return (
                    <tr key={`${j.job_id}-${j.repository}`} className={j.archived ? 'opacity-50' : ''}>
                      <td className="whitespace-nowrap" title={j.job_id}>
                        <button className="link-btn font-mono text-[13px] inline-block align-bottom whitespace-nowrap"
                          onClick={() => navigate(`/app/jobs/group/${encodeURIComponent(j.job_id)}`, { state: { backTo, backLabel: 'Back to repos' } })}>
                          {j.job_id}
                        </button>
                      </td>
                      <td className="hidden sm:table-cell">{timeAgo(j.last_seen)}</td>
                      <td className="hidden sm:table-cell">{j.run_count}</td>
                      <td className="whitespace-nowrap"><StatusBadge row={j} snoozed={snoozed} /></td>
                      <td className="hidden md:table-cell whitespace-nowrap">{j.monthly_savings_usd > 0 ? formatFromUSD(j.monthly_savings_usd, currency, { minimumFractionDigits: 0, maximumFractionDigits: 0 }) : '—'}</td>
                      <td className="hidden md:table-cell whitespace-nowrap"><span className="badge" style={benefitStyle(rowBenefit(j).tone)}>{rowBenefit(j).label}</span></td>
                      <td className="hidden md:table-cell whitespace-nowrap">
                        {j.latest_recommendations?.[0]?.spot_risk ? (
                          <span className="badge" style={spotStyle(j.latest_recommendations[0].spot_risk!)}>
                            {j.latest_recommendations[0].spot_risk}
                          </span>
                        ) : (
                          <span className="text-[var(--border)]">—</span>
                        )}
                      </td>
                      <td className="text-right">
                        <div className="flex gap-1 whitespace-nowrap justify-end">
                          {!j.archived && (
                            <button title={snoozed ? 'Manage snooze' : 'Snooze stale alerts'}
                              onClick={() => setSnoozeTarget(j)}
                              className="border border-[var(--border)] rounded px-2 py-1 text-xs bg-transparent cursor-pointer hover:bg-[var(--cream-alt)] transition-colors whitespace-nowrap">
                              {snoozed ? 'Unsnooze' : 'Snooze'}
                            </button>
                          )}
                          <button title={j.archived ? 'Unarchive' : 'Archive'} onClick={() => handleArchive(j)}
                            className="border border-[var(--border)] rounded px-2 py-1 text-xs bg-transparent cursor-pointer hover:bg-[var(--cream-alt)] transition-colors whitespace-nowrap">
                            {j.archived ? 'Unarchive' : 'Archive'}
                          </button>
                          <button title="Delete all runs" onClick={() => setConfirmDelete(j)}
                            className="border border-[var(--border)] rounded px-2 py-1 text-xs bg-transparent cursor-pointer hover:bg-red/10 hover:border-red/40 transition-colors whitespace-nowrap">
                            Delete
                          </button>
                        </div>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </div>

        <p className="text-xs text-[var(--text-light)] mt-3 mb-1">
          <strong>Snooze</strong> hides stale pressure temporarily for this job, <strong>Archive</strong> hides it from default lists, and <strong>Delete</strong> permanently removes all run history for this job.
        </p>
        </>
      )}

      {/* Snooze modal */}
      {snoozeTarget && <SnoozeModal row={snoozeTarget} onClose={() => setSnoozeTarget(null)} onSaved={load} />}

      {/* Delete confirmation */}
      {confirmDelete && (
        <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-[200] px-4" onClick={() => setConfirmDelete(null)}>
          <div className="bg-paper border border-[var(--border)] shadow-rr-lg rounded-xl p-7 w-full max-w-sm" onClick={e => e.stopPropagation()}>
            <h2 className="font-serif text-[1.1rem] font-bold text-[var(--text)] mb-3">Delete all runs?</h2>
            <p className="text-sm text-[var(--text-mid)] mb-5">
              Permanently delete all run history for <strong>{confirmDelete.job_id}</strong>. This cannot be undone.
            </p>
            <div className="flex gap-2 justify-end">
              <button onClick={handleDelete} disabled={deleting}
                className="btn-rr !bg-red text-sm py-2 px-4 disabled:opacity-50">{deleting ? 'Deleting…' : 'Delete'}</button>
              <button onClick={() => setConfirmDelete(null)}
                className="px-4 py-2 bg-[var(--cream-alt)] border border-[var(--border)] text-[var(--text-mid)] text-sm font-sans cursor-pointer">Cancel</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
