import { useEffect, useMemo, useState } from 'react'
import { useParams, useNavigate, useLocation } from 'react-router-dom'
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from 'recharts'
import { fetchJobs } from '../api'
import type { Job } from '../types'
import { DateRangePicker, inDateRange, EMPTY_RANGE } from '../components/DateRangePicker'
import type { DateRange } from '../components/DateRangePicker'

function median(values: number[]): number {
  if (values.length === 0) return 0
  const sorted = [...values].sort((a, b) => a - b)
  const mid = Math.floor(sorted.length / 2)
  return sorted.length % 2 === 0 ? (sorted[mid - 1] + sorted[mid]) / 2 : sorted[mid]
}

function deltaClass(pct: number) {
  if (pct < -0.5) return 'delta-negative'
  if (pct > 0.5)  return 'delta-positive'
  return 'delta-neutral'
}
function formatDelta(pct: number) {
  return `${pct > 0 ? '+' : ''}${pct.toFixed(1)}%`
}

function StatCard({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="stat-card" style={{ flex: '1 1 150px', textAlign: 'center', padding: '16px 12px' }}>
      <div className="stat-label">{label}</div>
      <div className="stat-value" style={{ fontSize: 22 }}>{value}</div>
      {sub && <div style={{ fontFamily: 'Lato, sans-serif', fontSize: 11, color: '#9A7B5A', marginTop: 4 }}>{sub}</div>}
    </div>
  )
}

export default function JobGroupPage() {
  const { jobId }  = useParams<{ jobId: string }>()
  const navigate   = useNavigate()
  const location   = useLocation() as { state?: { backTo?: string; backLabel?: string } }
  const [jobs, setJobs]       = useState<Job[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError]     = useState<string | null>(null)
  const [dateRange, setDateRange] = useState<DateRange>(EMPTY_RANGE)

  const decodedId = decodeURIComponent(jobId ?? '')

  useEffect(() => {
    fetchJobs()
      .then(setJobs)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  const allRuns = useMemo(
    () => jobs
      .filter(j => j.job_id === decodedId)
      .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()),
    [jobs, decodedId]
  )

  const runs = useMemo(
    () => allRuns.filter(r => inDateRange(r.created_at, dateRange)),
    [allRuns, dateRange]
  )

  const [page, setPage]         = useState(1)
  const [pageSize, setPageSize] = useState(10)

  useEffect(() => { setPage(1) }, [dateRange, pageSize])

  if (loading) return <div className="empty">Loading…</div>
  if (error)   return <div className="empty">Error: {error}</div>
  if (!allRuns.length) return <div className="empty">No runs found for "{decodedId}".</div>

  const cpuValues  = runs.map(r => r.summary?.cpu_percent_p95 ?? 0).filter(v => v > 0)
  const memValues  = runs.map(r => r.summary?.mem_used_gib_p95 ?? 0).filter(v => v > 0)
  const durValues  = runs.map(r => r.duration_seconds ?? 0).filter(v => v > 0)
  const costDeltas = runs.map(r => r.recommendations?.[0]?.cost_delta_percent ?? 0)

  const medCpu  = median(cpuValues)
  const medMem  = median(memValues)
  const medDur  = median(durValues)
  const avgCost = costDeltas.length ? costDeltas.reduce((a, b) => a + b, 0) / costDeltas.length : 0

  const latest   = (runs[0] ?? allRuns[0])
  const detected = latest?.summary?.detected_machine
  const topRec   = latest?.recommendations?.[0]
  const backTo   = location.state?.backTo ?? '/app'
  const backLabel = location.state?.backLabel ?? 'Back'

  const totalPages = Math.max(1, Math.ceil(runs.length / pageSize))
  const paginated  = runs.slice((page - 1) * pageSize, page * pageSize)

  return (
    <div className="fadein">
      {/* Breadcrumb header */}
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 10, marginBottom: 20 }}>
        <button
          onClick={() => navigate(backTo)}
          style={{
            background: 'none', border: 'none', color: '#9A7B5A', cursor: 'pointer',
            fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 13, letterSpacing: 1, padding: 0, whiteSpace: 'nowrap',
          }}
        >
          {backLabel}
        </button>
        <span aria-hidden="true" style={{ color: '#D4B896' }}>/</span>
        <h1 style={{ margin: 0, fontFamily: "var(--serif)", fontSize: 22, fontWeight: 900, color: "var(--text)", wordBreak: 'break-word' }}>{decodedId}</h1>
        <span style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 13, letterSpacing: 1, color: '#9A7B5A', whiteSpace: 'nowrap' }}>
          {runs.length !== allRuns.length
            ? <>{runs.length} of {allRuns.length} runs</>  
            : <>{allRuns.length} {allRuns.length === 1 ? 'run' : 'runs'}</>}
        </span>
      </div>

      {/* Date range picker */}
      <div style={{ marginBottom: 20 }}>
        <DateRangePicker value={dateRange} onChange={setDateRange} />
      </div>

      {runs.length === 0 && (
        <div className="empty" style={{ marginBottom: 24 }}>No runs in this time window.</div>
      )}

      {/* Aggregate stat cards */}
      <div className="stats-row" style={{ marginBottom: 24 }}>
        <StatCard
          label="Median CPU Usage"
          value={`${medCpu.toFixed(1)}%`}
          sub="p95 per run → median"
        />
        <StatCard
          label="Median Memory"
          value={`${medMem.toFixed(2)} GiB`}
          sub="p95 per run → median"
        />
        <StatCard
          label="Median Duration"
          value={`${medDur.toFixed(0)}s`}
        />
        <StatCard
          label="Avg Savings"
          value={formatDelta(avgCost)}
          sub="vs current machine"
        />
        {detected && (
          <StatCard
            label="Running On"
            value={detected.id}
            sub={detected.provider.toUpperCase()}
          />
        )}
        {topRec && (
          <StatCard
            label="Best Fit"
            value={topRec.machine.id}
            sub={topRec.machine.provider.toUpperCase()}
          />
        )}
      </div>

      {/* What p95 means */}
      <p style={{ fontSize: 12, color: '#9A7B5A', fontFamily: 'Lato, sans-serif', marginBottom: 16 }}>
        <strong>p95</strong> is the 95th-percentile sample within a single run — the load level sustained through
        95% of the job, filtering out momentary spikes. The medians above are computed from those p95 values across all runs.
      </p>

      {/* Trend charts — only render when there are ≥2 runs */}
      {runs.length >= 2 && (() => {
        const trendData = [...runs].reverse().map((r, i) => ({
          run: `#${runs.length - i}`,
          date: new Date(r.start_time).toLocaleDateString(),
          cpu: Number((r.summary?.cpu_percent_p95 ?? 0).toFixed(1)),
          mem: Number((r.summary?.mem_used_gib_p95 ?? 0).toFixed(2)),
          dur: Number((r.duration_seconds ?? 0).toFixed(0)),
        }))
        const tooltipStyle = { background: '#FFFDF7', border: '1px solid #D4B896', fontFamily: 'Lato', color: '#2C1A0E' }
        const axisStyle = { stroke: '#9A7B5A' }
        const tickStyle = { fontFamily: "'Lato'", fontSize: 11 }
        return (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))', gap: 16, marginBottom: 16 }}>
            <div className="card" style={{ marginBottom: 0 }}>
              <h2 style={{ marginBottom: 12 }}>CPU p95 Trend</h2>
              <div className="chart-wrap">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={trendData}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#D4B896" />
                    <XAxis dataKey="run" {...axisStyle} tick={tickStyle} />
                    <YAxis domain={[0, 100]} {...axisStyle} unit="%" tick={tickStyle} />
                    <Tooltip contentStyle={tooltipStyle} formatter={(v) => [`${Number(v).toFixed(1)}%`, 'CPU p95']} />
                    <Line type="monotone" dataKey="cpu" stroke="#C23B22" strokeWidth={2} dot={{ r: 3 }} name="CPU p95 %" />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            </div>
            <div className="card" style={{ marginBottom: 0 }}>
              <h2 style={{ marginBottom: 12 }}>Memory p95 Trend</h2>
              <div className="chart-wrap">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={trendData}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#D4B896" />
                    <XAxis dataKey="run" {...axisStyle} tick={tickStyle} />
                    <YAxis {...axisStyle} unit=" GiB" tick={tickStyle} />
                    <Tooltip contentStyle={tooltipStyle} formatter={(v) => [`${Number(v).toFixed(2)} GiB`, 'Mem p95']} />
                    <Line type="monotone" dataKey="mem" stroke="#1B3361" strokeWidth={2} dot={{ r: 3 }} name="Mem p95 GiB" />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            </div>
            <div className="card" style={{ marginBottom: 0 }}>
              <h2 style={{ marginBottom: 12 }}>Duration Trend</h2>
              <div className="chart-wrap">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={trendData}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#D4B896" />
                    <XAxis dataKey="run" {...axisStyle} tick={tickStyle} />
                    <YAxis {...axisStyle} unit="s" tick={tickStyle} />
                    <Tooltip contentStyle={tooltipStyle} formatter={(v) => [`${Number(v).toFixed(0)}s`, 'Duration']} />
                    <Line type="monotone" dataKey="dur" stroke="#B8860B" strokeWidth={2} dot={{ r: 3 }} name="Duration (s)" />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            </div>
          </div>
        )
      })()}

      {/* Run history table */}
      <div className="sm:hidden space-y-3 mb-4">
        {paginated.map((run, i) => {
          const rec    = run.recommendations?.[0]
          const delta  = rec?.cost_delta_percent ?? 0
          const runNum = runs.length - ((page - 1) * pageSize + i)
          return (
            <button
              key={run.id}
              className="w-full text-left bg-paper border border-[var(--border)] rounded-lg px-4 py-3 shadow-rr"
              onClick={() => navigate(`/app/jobs/${run.id}`)}
            >
              <div className="flex items-start justify-between gap-3 mb-2">
                <div>
                  <div className="font-deco text-[12px] tracking-[1px] text-[var(--text-light)]">RUN #{runNum}</div>
                  <div className="font-mono text-[13px] text-[var(--text)] break-all mt-1">{run.summary?.detected_machine?.id}</div>
                </div>
                {rec ? <span className={`badge badge-${rec.tier}`}>{rec.tier}</span> : null}
              </div>
              <div className="grid grid-cols-2 gap-2 text-xs text-[var(--text-mid)]">
                <div><span className="text-[var(--text-light)]">Savings</span><div className={deltaClass(delta)}>{formatDelta(delta)}</div></div>
                <div><span className="text-[var(--text-light)]">Best Fit</span><div className="font-mono break-all">{rec?.machine.id}</div></div>
                <div><span className="text-[var(--text-light)]">CPU</span><div>{run.summary?.cpu_percent_p95 != null ? `${run.summary.cpu_percent_p95.toFixed(1)}%` : '—'}</div></div>
                <div><span className="text-[var(--text-light)]">Mem</span><div>{run.summary?.mem_used_gib_p95 != null ? `${run.summary.mem_used_gib_p95.toFixed(2)} GiB` : '—'}</div></div>
                <div><span className="text-[var(--text-light)]">Duration</span><div>{run.duration_seconds != null ? `${run.duration_seconds.toFixed(0)}s` : '—'}</div></div>
                <div><span className="text-[var(--text-light)]">Date</span><div>{new Date(run.created_at).toLocaleDateString()}</div></div>
              </div>
            </button>
          )
        })}
      </div>

      <div className="card hidden sm:block">
        <div className="table-wrap">
          <table className="rr-table">
            <thead>
              <tr>
                <th style={{ width: 48 }}>#</th>
                <th>Running On</th>
                <th>Best Fit</th>
                <th>Tier</th>
                <th>Savings</th>
                <th>CPU p95</th>
                <th>Mem p95</th>
                <th>Duration</th>
                <th>Date</th>
              </tr>
            </thead>
            <tbody>
              {paginated.map((run, i) => {
                const rec    = run.recommendations?.[0]
                const delta  = rec?.cost_delta_percent ?? 0
                const runNum = runs.length - ((page - 1) * pageSize + i)
                return (
                  <tr key={run.id} style={{ cursor: 'pointer' }} onClick={() => navigate(`/app/jobs/${run.id}`)}>
                    <td style={{ color: '#9A7B5A', fontFamily: "'Bebas Neue'", fontSize: 13 }}>
                      {runNum}
                    </td>
                    <td style={{ fontFamily: 'monospace', fontSize: 13 }}>{run.summary?.detected_machine?.id}</td>
                    <td style={{ fontFamily: 'monospace', fontSize: 13 }}>{rec?.machine.id}</td>
                    <td>{rec ? <span className={`badge badge-${rec.tier}`}>{rec.tier}</span> : null}</td>
                    <td>{rec ? <span className={deltaClass(delta)}>{formatDelta(delta)}</span> : null}</td>
                    <td>{run.summary?.cpu_percent_p95 != null ? `${run.summary.cpu_percent_p95.toFixed(1)}%` : null}</td>
                    <td>{run.summary?.mem_used_gib_p95 != null ? `${run.summary.mem_used_gib_p95.toFixed(2)} GiB` : null}</td>
                    <td>{run.duration_seconds != null ? `${run.duration_seconds.toFixed(0)}s` : null}</td>
                    <td>{new Date(run.created_at).toLocaleDateString()}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      </div>

      {(totalPages > 1 || runs.length > 0) && (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 12, marginTop: 12 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 12, letterSpacing: 1.5, color: '#9A7B5A' }}>ROWS</span>
            {PAGE_SIZES.map(s => (
              <button key={s} onClick={() => setPageSize(s)} style={{
                background:  pageSize === s ? '#2C1A0E' : 'transparent',
                color:       pageSize === s ? '#FBF0DC' : '#9A7B5A',
                border:      '1px solid',
                borderColor: pageSize === s ? '#2C1A0E' : '#D4B896',
                padding: '4px 10px',
                fontFamily: "'Bebas Neue', Impact, sans-serif",
                fontSize: 13, letterSpacing: 1, cursor: 'pointer', transition: 'all .1s',
              }}>{s}</button>
            ))}
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            <span style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 12, letterSpacing: 1, color: '#9A7B5A', marginRight: 8 }}>
              {(page - 1) * pageSize + 1}–{Math.min(page * pageSize, runs.length)} of {runs.length}
            </span>
            <button onClick={() => setPage(1)} disabled={page === 1} style={pgBtnStyle(page === 1)}>«</button>
            <button onClick={() => setPage(p => p - 1)} disabled={page === 1} style={pgBtnStyle(page === 1)}>‹</button>
            {pageNumbers(page, totalPages).map((p, i) =>
              p === null
                ? <span key={`e-${i}`} style={{ color: '#9A7B5A', padding: '0 4px', fontFamily: "'Bebas Neue'" }}>…</span>
                : <button key={p} onClick={() => setPage(p)} style={pgBtnStyle(false, p === page)}>{p}</button>
            )}
            <button onClick={() => setPage(p => p + 1)} disabled={page === totalPages} style={pgBtnStyle(page === totalPages)}>›</button>
            <button onClick={() => setPage(totalPages)} disabled={page === totalPages} style={pgBtnStyle(page === totalPages)}>»</button>
          </div>
        </div>
      )}
    </div>
  )
}

const PAGE_SIZES = [10, 20, 50]

function pgBtnStyle(disabled: boolean, active = false): React.CSSProperties {
  return {
    background:  active ? '#C23B22' : 'transparent',
    color:       active ? '#FBF0DC' : disabled ? '#D4B896' : '#6B4226',
    border:      '1px solid',
    borderColor: active ? '#C23B22' : '#D4B896',
    padding: '4px 10px',
    fontFamily: "'Bebas Neue', Impact, sans-serif",
    fontSize: 14,
    cursor:  disabled ? 'default' : 'pointer',
    opacity: disabled ? 0.4 : 1,
    minWidth: 32,
    transition: 'all .1s',
    boxShadow: active ? '2px 2px 0 rgba(92,58,30,.15)' : 'none',
  }
}

function pageNumbers(current: number, total: number): (number | null)[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1)
  const pages: (number | null)[] = [1]
  if (current > 3) pages.push(null)
  for (let p = Math.max(2, current - 1); p <= Math.min(total - 1, current + 1); p++) pages.push(p)
  if (current < total - 2) pages.push(null)
  pages.push(total)
  return pages
}
