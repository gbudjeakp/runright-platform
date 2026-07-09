import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'

// Types matching backend
interface RunSnapshot {
  id: number
  run_id: string
  job_id: string
  start_time: string
  duration_seconds: number
  cpu_percent_p95: number
  mem_used_gib_p95: number
  detected_machine?: string
  top_recommend?: string
  cost_delta_percent: number
  est_carbon_kg: number
}

export default function RunHistoryPage() {
  const [runs, setRuns] = useState<RunSnapshot[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [jobFilter, setJobFilter] = useState('')
  const [repoFilter, setRepoFilter] = useState('')

  useEffect(() => {
    loadRuns()
  }, [jobFilter, repoFilter])

  async function loadRuns() {
    setLoading(true)
    setError('')
    try {
      const params = new URLSearchParams({ limit: '100' })
      if (jobFilter) params.set('job_id', jobFilter)
      if (repoFilter) params.set('repository', repoFilter)
      
      const res = await fetch(`/api/v1/runs/history?${params}`, { credentials: 'include' })
      if (res.ok) {
        const data = await res.json()
        setRuns(data.runs || [])
      } else {
        setError('Failed to load recent activity')
      }
    } catch {
      setError('Failed to load recent activity')
    } finally {
      setLoading(false)
    }
  }

  function formatDuration(secs: number): string {
    if (secs < 60) return `${secs.toFixed(0)}s`
    if (secs < 3600) return `${(secs / 60).toFixed(1)}m`
    return `${(secs / 3600).toFixed(1)}h`
  }

  function formatDate(iso: string): string {
    return new Date(iso).toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    })
  }

  function relativeTime(iso: string): string {
    const diff = Date.now() - new Date(iso).getTime()
    const mins = Math.floor(diff / 60000)
    if (mins < 1) return 'just now'
    if (mins < 60) return `${mins}m ago`
    const hrs = Math.floor(mins / 60)
    if (hrs < 24) return `${hrs}h ago`
    const days = Math.floor(hrs / 24)
    return `${days}d ago`
  }

  return (
    <div className="fadein max-w-7xl">
      <div className="flex flex-wrap items-center justify-between gap-4 mb-6">
        <div>
          <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] tracking-tight">
            Recent Activity
          </h1>
          <p className="text-sm text-[var(--text-light)] mt-1">
            All CI runs across your organization
          </p>
        </div>
        <div className="flex gap-3">
          <input
            type="text"
            placeholder="Filter by job..."
            value={jobFilter}
            onChange={(e) => setJobFilter(e.target.value)}
            className="px-3 py-2 text-sm border border-[var(--border)] rounded-lg bg-paper text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--text-light)] w-40"
          />
          <input
            type="text"
            placeholder="Filter by repo..."
            value={repoFilter}
            onChange={(e) => setRepoFilter(e.target.value)}
            className="px-3 py-2 text-sm border border-[var(--border)] rounded-lg bg-paper text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--text-light)] w-40"
          />
        </div>
      </div>

      {error && (
        <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded mb-6">
          {error}
        </div>
      )}

      {loading ? (
        <div className="text-[var(--text-light)] py-12 text-center">Loading activity...</div>
      ) : runs.length === 0 ? (
        <div className="bg-paper border border-[var(--border)] rounded-lg p-12 text-center">
          <div className="text-[var(--text-light)] mb-2">No runs found</div>
          <p className="text-sm text-[var(--text-light)]">
            CI runs will appear here after your first job completes.
          </p>
        </div>
      ) : (
        <div className="space-y-3">
          {runs.map((run) => (
            <Link
              key={run.id}
              to={`/app/jobs/group/${encodeURIComponent(run.job_id)}`}
              className="block bg-paper border border-[var(--border)] rounded-lg p-4 hover:border-[var(--text-light)] transition-colors"
            >
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2 mb-1">
                    <span className="font-medium text-[var(--text)] truncate">
                      {run.job_id}
                    </span>
                    {run.cost_delta_percent < 0 && (
                      <span className="shrink-0 px-2 py-0.5 text-xs font-medium bg-green-100 text-green-800 rounded-full">
                        {run.cost_delta_percent.toFixed(0)}% savings
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-4 text-xs text-[var(--text-light)]">
                    <span>{relativeTime(run.start_time)}</span>
                    <span>•</span>
                    <span>{formatDuration(run.duration_seconds)}</span>
                    {run.detected_machine && (
                      <>
                        <span>•</span>
                        <span className="font-mono">{run.detected_machine}</span>
                      </>
                    )}
                  </div>
                </div>
                <div className="shrink-0 text-right">
                  <div className="flex items-center gap-3 text-sm">
                    <div>
                      <div className="text-xs text-[var(--text-light)]">CPU</div>
                      <div className={`font-mono ${run.cpu_percent_p95 > 80 ? 'text-[var(--red)]' : 'text-[var(--text)]'}`}>
                        {run.cpu_percent_p95.toFixed(0)}%
                      </div>
                    </div>
                    <div>
                      <div className="text-xs text-[var(--text-light)]">Mem</div>
                      <div className="font-mono text-[var(--text)]">
                        {run.mem_used_gib_p95.toFixed(1)}G
                      </div>
                    </div>
                  </div>
                  {run.top_recommend && run.top_recommend !== run.detected_machine && (
                    <div className="text-xs text-green-700 mt-1">
                      → {run.top_recommend}
                    </div>
                  )}
                </div>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}

function RunCard({ run, label, color }: { run: RunSnapshot; label: string; color: 'blue' | 'green' }) {
  const accentBorder = color === 'blue' ? 'border-l-[var(--navy)]' : 'border-l-[var(--gold)]'
  const accentText   = color === 'blue' ? 'text-[var(--navy)]'    : 'text-[var(--gold)]'

  return (
    <div className={`bg-[var(--paper)] border border-[var(--border)] border-l-4 ${accentBorder} rounded-lg p-4`}>
      <div className={`text-sm font-medium ${accentText} mb-3`}>{label} — Run #{run.id}</div>
      <div className="space-y-2 text-sm">
        <div className="flex justify-between">
          <span className="text-[var(--text-light)]">Duration</span>
          <span className="font-mono">{run.duration_seconds.toFixed(0)}s</span>
        </div>
        <div className="flex justify-between">
          <span className="text-[var(--text-light)]">CPU P95</span>
          <span className="font-mono">{run.cpu_percent_p95.toFixed(1)}%</span>
        </div>
        <div className="flex justify-between">
          <span className="text-[var(--text-light)]">Memory P95</span>
          <span className="font-mono">{run.mem_used_gib_p95.toFixed(2)} GiB</span>
        </div>
        <div className="flex justify-between">
          <span className="text-[var(--text-light)]">Machine</span>
          <span className="font-mono text-xs">{run.detected_machine || '-'}</span>
        </div>
        <div className="flex justify-between">
          <span className="text-[var(--text-light)]">Carbon</span>
          <span className="font-mono">{(run.est_carbon_kg * 1000).toFixed(1)}g CO₂</span>
        </div>
      </div>
    </div>
  )
}
