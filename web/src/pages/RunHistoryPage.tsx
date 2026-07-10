import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { usePagination } from '../hooks/usePagination'
import { ListControls } from '../components/ListControls'
import type { RunSnapshot, RunDiff, DiffInsight } from '../types'

export default function RunHistoryPage() {
  const [runs, setRuns] = useState<RunSnapshot[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [jobFilter, setJobFilter] = useState('')
  const [repoFilter, setRepoFilter] = useState('')
  
  // Comparison state
  const [selectedRuns, setSelectedRuns] = useState<Set<number>>(new Set())
  const [comparison, setComparison] = useState<RunDiff | null>(null)
  const [comparing, setComparing] = useState(false)
  const [showComparison, setShowComparison] = useState(false)

  // Pagination with search
  const pagination = usePagination({
    items: runs,
    pageSize: 20,
    searchFn: (run, query) =>
      run.job_id.toLowerCase().includes(query) ||
      (run.detected_machine?.toLowerCase().includes(query) ?? false) ||
      (run.top_recommend?.toLowerCase().includes(query) ?? false),
  })

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

  function toggleSelection(id: number) {
    setSelectedRuns(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else if (next.size < 2) {
        next.add(id)
      }
      return next
    })
  }

  async function compareRuns() {
    const ids = Array.from(selectedRuns).sort((a, b) => a - b)
    if (ids.length !== 2) return
    
    setComparing(true)
    try {
      const res = await fetch(`/api/v1/runs/compare?before=${ids[0]}&after=${ids[1]}`, { 
        credentials: 'include' 
      })
      if (res.ok) {
        const data: RunDiff = await res.json()
        setComparison(data)
        setShowComparison(true)
      } else {
        setError('Failed to compare runs')
      }
    } catch {
      setError('Failed to compare runs')
    } finally {
      setComparing(false)
    }
  }

  function formatDuration(secs: number): string {
    if (secs < 60) return `${secs.toFixed(0)}s`
    if (secs < 3600) return `${(secs / 60).toFixed(1)}m`
    return `${(secs / 3600).toFixed(1)}h`
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
            All CI runs across your organization • Select 2 runs to compare
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

      {/* Comparison bar */}
      {selectedRuns.size > 0 && (
        <div className="bg-[var(--cream-alt)] border border-[var(--border)] rounded-lg p-3 mb-4 flex items-center justify-between">
          <span className="text-sm text-[var(--text)]">
            {selectedRuns.size} run{selectedRuns.size > 1 ? 's' : ''} selected
          </span>
          <div className="flex gap-2">
            <button
              onClick={() => setSelectedRuns(new Set())}
              className="px-3 py-1.5 text-xs border border-[var(--border)] rounded bg-[var(--paper)] hover:bg-[var(--cream-alt)]"
            >
              Clear
            </button>
            <button
              onClick={compareRuns}
              disabled={selectedRuns.size !== 2 || comparing}
              className="px-3 py-1.5 text-xs bg-[var(--red)] text-white rounded disabled:opacity-50"
            >
              {comparing ? 'Comparing...' : 'Compare'}
            </button>
          </div>
        </div>
      )}

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
        <div className="space-y-4">
          <ListControls 
            pagination={pagination} 
            searchPlaceholder="Search runs..."
            pageSizeOptions={[10, 20, 50, 100]}
          />
          <div className="space-y-3">
            {pagination.paginatedItems.map((run) => (
              <div
                key={run.id}
                className={`bg-paper border rounded-lg p-4 transition-colors ${
                  selectedRuns.has(run.id) 
                    ? 'border-[var(--red)] ring-1 ring-[var(--red)]' 
                    : 'border-[var(--border)] hover:border-[var(--text-light)]'
                }`}
              >
                <div className="flex items-start gap-3">
                  <input
                    type="checkbox"
                    checked={selectedRuns.has(run.id)}
                    onChange={() => toggleSelection(run.id)}
                    disabled={!selectedRuns.has(run.id) && selectedRuns.size >= 2}
                    className="mt-1 w-4 h-4 accent-[var(--red)]"
                  />
                  <Link
                    to={`/app/jobs/group/${encodeURIComponent(run.job_id)}`}
                    className="flex-1 min-w-0"
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
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Comparison Modal */}
      {showComparison && comparison && (
        <ComparisonModal 
          diff={comparison} 
          onClose={() => { setShowComparison(false); setComparison(null); setSelectedRuns(new Set()); }} 
        />
      )}
    </div>
  )
}

// Comparison Modal
function ComparisonModal({ diff, onClose }: { diff: RunDiff; onClose: () => void }) {
  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50 p-4">
      <div className="bg-[var(--paper)] rounded-lg max-w-4xl w-full max-h-[90vh] overflow-y-auto">
        <div className="border-b border-[var(--border)] p-4 flex items-center justify-between sticky top-0 bg-[var(--paper)]">
          <h2 className="font-serif text-xl font-bold">Run Comparison</h2>
          <button onClick={onClose} className="text-[var(--text-light)] hover:text-[var(--text)] text-2xl">&times;</button>
        </div>
        
        <div className="p-6">
          {/* Side by side run cards */}
          <div className="grid grid-cols-2 gap-4 mb-6">
            <RunCard run={diff.before} label="Before (Baseline)" color="blue" />
            <RunCard run={diff.after} label="After (Current)" color="green" />
          </div>

          {/* Delta summary */}
          <div className="bg-[var(--cream-alt)] rounded-lg p-4 mb-6">
            <h3 className="font-medium text-sm text-[var(--text-light)] mb-3 uppercase tracking-wide">Changes</h3>
            <div className="grid grid-cols-5 gap-4 text-center">
              <DeltaCard label="CPU" value={diff.cpu_delta_percent} unit="%" isPercent />
              <DeltaCard label="Memory" value={diff.memory_delta_gib} unit=" GiB" />
              <DeltaCard label="Duration" value={diff.duration_delta_seconds} unit="s" />
              <DeltaCard label="Cost" value={diff.cost_delta_usd} unit="%" isPercent />
              <DeltaCard label="Carbon" value={diff.carbon_delta_kg * 1000} unit="g" />
            </div>
          </div>

          {/* Insights */}
          {diff.insights && diff.insights.length > 0 && (
            <div>
              <h3 className="font-medium text-sm text-[var(--text-light)] mb-3 uppercase tracking-wide">Insights</h3>
              <div className="space-y-2">
                {diff.insights.map((insight, i) => (
                  <InsightCard key={i} insight={insight} />
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
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

function DeltaCard({ label, value, unit, isPercent }: { label: string; value: number; unit: string; isPercent?: boolean }) {
  const isNegative = value < -0.01
  const isPositive = value > 0.01
  const color = isNegative ? 'text-green-600' : isPositive ? 'text-red-600' : 'text-[var(--text-light)]'
  const sign = isPositive ? '+' : ''
  
  return (
    <div>
      <div className="text-xs text-[var(--text-light)] mb-1">{label}</div>
      <div className={`font-mono text-lg font-bold ${color}`}>
        {sign}{value.toFixed(isPercent ? 1 : 2)}{unit}
      </div>
    </div>
  )
}

function InsightCard({ insight }: { insight: DiffInsight }) {
  const styles = {
    warning: 'bg-amber-50 border-amber-200 text-amber-800',
    improvement: 'bg-green-50 border-green-200 text-green-800',
    neutral: 'bg-slate-50 border-slate-200 text-slate-800',
  }
  const icons = {
    warning: '⚠️',
    improvement: '✅',
    neutral: 'ℹ️',
  }
  
  return (
    <div className={`border rounded-lg p-3 text-sm ${styles[insight.type]}`}>
      <span className="mr-2">{icons[insight.type]}</span>
      {insight.message}
    </div>
  )
}
