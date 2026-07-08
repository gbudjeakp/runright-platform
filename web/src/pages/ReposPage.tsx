import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { AxiosError } from 'axios'
import { fetchRepos, fetchIsolatedJobs } from '../api'
import type { RepoSummary, JobSummaryRow } from '../types'
import { formatFromUSD, useCurrencyPreference } from '../currency'

function timeAgo(iso: string) {
  const secs = Math.floor((Date.now() - new Date(iso).getTime()) / 1000)
  if (secs < 60) return `${secs}s ago`
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`
  return `${Math.floor(secs / 86400)}d ago`
}

export default function ReposPage() {
  const { currency } = useCurrencyPreference()
  const navigate = useNavigate()
  const [repos, setRepos] = useState<RepoSummary[]>([])
  const [isolated, setIsolated] = useState<JobSummaryRow[]>([])
  const [search, setSearch] = useState('')
  const [repoInput, setRepoInput] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    Promise.allSettled([fetchRepos(), fetchIsolatedJobs()])
      .then(([reposResult, isolatedResult]) => {
        let nextError = ''

        if (reposResult.status === 'fulfilled') {
          setRepos(reposResult.value)
        } else {
          const e = reposResult.reason
          if (e instanceof AxiosError) {
            if (e.response?.status === 404) {
              nextError = 'Repository endpoint not found. Update or restart the backend API.'
            } else {
              nextError = e.message
            }
          } else {
            nextError = String(e)
          }
        }

        if (isolatedResult.status === 'fulfilled') {
          setIsolated(isolatedResult.value)
          return
        }

        // Older API builds may not expose isolated jobs yet; keep page usable.
        const e = isolatedResult.reason
        if (e instanceof AxiosError && e.response?.status === 404) {
          setIsolated([])
          if (nextError) setError(nextError)
          return
        }

        if (!nextError) nextError = e instanceof AxiosError ? e.message : String(e)
        if (nextError) setError(nextError)
      })
      .finally(() => setLoading(false))
  }, [])

  if (loading) return <div className="empty">Loading repositories…</div>

  const filteredRepos = repos.filter((r) =>
    r.repository.toLowerCase().includes(search.trim().toLowerCase()),
  )
  const trimmedRepoInput = repoInput.trim()
  const canOpenRepo = trimmedRepoInput.length > 0

  function openRepoFromInput() {
    if (!canOpenRepo) return
    navigate(`/app/repos/detail?repo=${encodeURIComponent(trimmedRepoInput)}`)
  }

  const totalSavings = repos.reduce((s, r) => s + r.monthly_savings_usd, 0)
  const totalStale   = repos.reduce((s, r) => s + r.stale_count, 0)

  return (
    <div className="fadein max-w-[1100px]">
      <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] mb-7 tracking-tight">Repositories</h1>

      {error && (
        <div className="mb-5 rounded-md border border-red/30 bg-red/10 px-4 py-3 text-sm text-red">
          {error}
        </div>
      )}

      <div className="rr-card !p-4 sm:!p-5 mb-6">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div className="flex-1 max-w-[520px] min-w-0">
            <label className="block font-deco text-[11px] tracking-[1.5px] text-[var(--text-mid)] uppercase mb-1.5">
              Open repository
            </label>
            <div className="flex flex-col sm:flex-row gap-2">
              <input
                type="text"
                value={repoInput}
                onChange={(e) => setRepoInput(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') openRepoFromInput() }}
                className="rr-input"
                placeholder="owner/repository"
              />
              <button
                onClick={openRepoFromInput}
                disabled={!canOpenRepo}
                className="btn-rr whitespace-nowrap disabled:opacity-50"
              >
                Open Repo
              </button>
            </div>
            <p className="text-xs text-[var(--text-light)] mt-2">
              Repositories are discovered from incoming job runs. Use this to jump directly when you know the repo slug.
            </p>
          </div>
          <div className="w-full lg:w-[300px] min-w-0">
            <label className="block font-deco text-[11px] tracking-[1.5px] text-[var(--text-mid)] uppercase mb-1.5">
              Filter tracked repos
            </label>
            <input
              type="text"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="rr-input"
              placeholder="Search by owner or repo"
            />
          </div>
        </div>
      </div>

      {/* Summary banner */}
      {repos.length > 0 && (
        <div className="flex flex-wrap gap-6 sm:gap-10 items-center rounded-md px-5 py-4 mb-6"
          style={{ background: 'linear-gradient(90deg,#2C1A0E,#3D2510)' }}>
          <div className="flex flex-col items-center">
            <span className="font-deco text-3xl" style={{ color: '#E8C458' }}>{repos.length}</span>
            <span className="font-deco text-[11px] tracking-[1.5px]" style={{ color: '#C4A882' }}>REPOS TRACKED</span>
          </div>
          <div className="flex flex-col items-center">
            <span className="font-deco text-3xl" style={{ color: '#E8C458' }}>{formatFromUSD(totalSavings, currency, { minimumFractionDigits: 0, maximumFractionDigits: 0 })}</span>
            <span className="font-deco text-[11px] tracking-[1.5px]" style={{ color: '#C4A882' }}>SAVINGS/MO</span>
          </div>
          {totalStale > 0 && (
            <div className="flex flex-col items-center">
              <span className="font-deco text-3xl text-[#E8A838]">{totalStale}</span>
              <span className="font-deco text-[11px] tracking-[1.5px]" style={{ color: '#C4A882' }}>STALE JOBS</span>
            </div>
          )}
        </div>
      )}

      {repos.length === 0 && isolated.length === 0 ? (
        <div className="empty">
          No job data yet. Run the RunRight action in a GitHub workflow to start tracking.
        </div>
      ) : repos.length === 0 ? (
        <div className="rr-card text-[var(--text-mid)]">
          <h2 className="font-serif text-[19px] font-bold text-[var(--text)] mb-2">No repositories tagged yet</h2>
          <p className="mb-2">We found job runs, but they were not associated with a repository.</p>
          <p className="text-sm text-[var(--text-light)]">Tip: set repository metadata in your CI run so jobs are grouped under owner/repository.</p>
        </div>
      ) : (
        <>
          {/* Repo cards grid */}
          <div className="hidden sm:grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4 mb-10">
            {filteredRepos.map((r) => (
              <button
                key={r.repository}
                className="text-left cursor-pointer bg-paper border border-[var(--border)] rounded-lg px-5 py-4 shadow-rr transition-shadow hover:shadow-rr-lg hover:border-[var(--navy)] w-full"
                onClick={() => navigate(`/app/repos/detail?repo=${encodeURIComponent(r.repository)}`)}
              >
                <div className="flex justify-between items-baseline mb-2.5">
                  <span className="font-sans font-semibold text-sm text-[var(--text)] break-all leading-snug pr-2">{r.repository}</span>
                  <span className="font-sans text-xs text-[var(--text-light)] shrink-0 whitespace-nowrap ml-2">{timeAgo(r.last_seen)}</span>
                </div>
                <div className="flex flex-wrap gap-2 text-sm text-[var(--text-mid)]">
                  <span>{r.job_count} job{r.job_count !== 1 ? 's' : ''}</span>
                  <span className={r.monthly_savings_usd > 0 ? 'text-[#2E7D32] font-semibold' : 'text-[var(--text-light)]'}>
                    {formatFromUSD(r.monthly_savings_usd, currency, { minimumFractionDigits: 0, maximumFractionDigits: 0 })}/mo
                  </span>
                  {r.stale_count > 0 && (
                    <span className="badge" style={{ background: '#fef3c7', color: '#92400e', borderColor: '#92400e' }}>
                      {r.stale_count} stale
                    </span>
                  )}
                  {r.snoozed_count > 0 && (
                    <span className="badge" style={{ background: '#e0e7ff', color: '#3730a3', borderColor: '#3730a3' }}>
                      {r.snoozed_count} snoozed
                    </span>
                  )}
                </div>
              </button>
            ))}
          </div>

          {repos.length > 0 && filteredRepos.length === 0 && (
            <div className="empty text-base !py-10">No repositories match your filter.</div>
          )}

          <div className="sm:hidden space-y-3 mb-8">
            {filteredRepos.map((r) => (
              <button
                key={`${r.repository}-mobile`}
                className="w-full text-left bg-paper border border-[var(--border)] rounded-lg px-4 py-3 shadow-rr"
                onClick={() => navigate(`/app/repos/detail?repo=${encodeURIComponent(r.repository)}`)}
              >
                <div className="flex items-start justify-between gap-3 mb-2">
                  <div>
                    <div className="font-sans font-semibold text-sm text-[var(--text)] break-all leading-snug pr-2">{r.repository}</div>
                    <div className="text-xs text-[var(--text-light)] mt-1">{timeAgo(r.last_seen)}</div>
                  </div>
                  <span className="badge badge-github">Repo</span>
                </div>
                <div className="flex flex-wrap gap-2 text-xs text-[var(--text-mid)]">
                  <span>{r.job_count} job{r.job_count !== 1 ? 's' : ''}</span>
                  <span className={r.monthly_savings_usd > 0 ? 'text-[#2E7D32] font-semibold' : 'text-[var(--text-light)]'}>
                    {formatFromUSD(r.monthly_savings_usd, currency, { minimumFractionDigits: 0, maximumFractionDigits: 0 })}/mo
                  </span>
                  {r.stale_count > 0 && <span>{r.stale_count} stale</span>}
                  {r.snoozed_count > 0 && <span>{r.snoozed_count} snoozed</span>}
                </div>
              </button>
            ))}
          </div>

          {/* Isolated jobs */}
          {isolated.length > 0 && (
            <section>
              <h2 className="font-serif text-[17px] font-bold text-[var(--text)] mb-4">Jobs without a repository</h2>
              <div className="sm:hidden space-y-3 mb-4">
                {isolated.map((j) => (
                  <button
                    key={`${j.job_id}-mobile`}
                    className="w-full text-left bg-paper border border-[var(--border)] rounded-lg px-4 py-3 shadow-rr"
                    onClick={() => navigate(`/app/repos/detail?isolated=true&job=${encodeURIComponent(j.job_id)}`)}
                  >
                    <div className="font-mono text-[13px] text-[var(--text)] break-all">{j.job_id}</div>
                    <div className="flex justify-between gap-3 text-xs text-[var(--text-light)] mt-2">
                      <span>{timeAgo(j.last_seen)}</span>
                      <span>{j.run_count} runs</span>
                    </div>
                  </button>
                ))}
              </div>
              <div className="rr-card !p-0 hidden sm:block">
                <div className="table-wrap">
                  <table className="rr-table min-w-[620px] table-fixed">
                    <colgroup>
                      <col style={{ width: '42%' }} />
                      <col style={{ width: '18%' }} />
                      <col style={{ width: '12%' }} />
                      <col style={{ width: '12%' }} />
                      <col style={{ width: '16%' }} />
                    </colgroup>
                    <thead>
                      <tr>
                        <th>Job ID</th>
                        <th>Last seen</th>
                        <th className="hidden sm:table-cell">Runs</th>
                        <th>Stale</th>
                        <th className="hidden sm:table-cell">Savings/mo</th>
                      </tr>
                    </thead>
                    <tbody>
                      {isolated.map((j) => (
                        <tr
                          key={j.job_id}
                          className="cursor-pointer"
                          onClick={() => navigate(`/app/repos/detail?isolated=true&job=${encodeURIComponent(j.job_id)}`)}
                        >
                          <td className="font-mono text-[13px] truncate" title={j.job_id}>{j.job_id}</td>
                          <td>{timeAgo(j.last_seen)}</td>
                          <td className="hidden sm:table-cell">{j.run_count}</td>
                          <td>
                            {j.stale
                              ? <span className="badge" style={{ background: '#fef3c7', color: '#92400e', borderColor: '#92400e' }}>stale</span>
                              : '—'}
                          </td>
                          <td className="hidden sm:table-cell">
                            {j.monthly_savings_usd > 0 ? formatFromUSD(j.monthly_savings_usd, currency, { minimumFractionDigits: 0, maximumFractionDigits: 0 }) : '—'}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </section>
          )}
        </>
      )}
    </div>
  )
}
