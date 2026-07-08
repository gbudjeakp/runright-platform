import { useEffect, useState } from 'react'
import { useParams, Link, useNavigate } from 'react-router-dom'
import {
  XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid, Legend,
  BarChart, Bar,
} from 'recharts'
import { fetchJob } from '../api'
import { formatFromUSD, useCurrencyPreference } from '../currency'
import type { Job, Recommendation } from '../types'

function TierBadge({ tier }: { tier: string }) {
  return <span className={`badge badge-${tier}`}>{tier}</span>
}

function DeltaCell({ pct }: { pct: number }) {
  const cls = pct < -0.5 ? 'delta-negative' : pct > 0.5 ? 'delta-positive' : 'delta-neutral'
  const sign = pct > 0 ? '+' : ''
  return <span className={cls}>{sign}{pct.toFixed(1)}%</span>
}

function detectionTone(level?: string): React.CSSProperties {
  if (level === 'high') return { color: '#2E7D32', borderColor: '#2E7D32', background: 'rgba(46,125,50,.08)' }
  if (level === 'medium') return { color: '#9A7B5A', borderColor: '#9A7B5A', background: 'rgba(154,123,90,.10)' }
  if (level === 'low') return { color: '#C23B22', borderColor: '#C23B22', background: 'rgba(194,59,34,.08)' }
  return { color: '#9A7B5A', borderColor: '#D4B896', background: 'rgba(212,184,150,.12)' }
}

export default function JobDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { currency } = useCurrencyPreference()
  const [job, setJob] = useState<Job | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!id) return
    fetchJob(Number(id))
      .then(setJob)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [id])

  if (loading) return <div className="empty">Loading job…</div>
  if (error || !job) return <div className="empty">Job not found: {error}</div>

  const s = job.summary
  const recs: Recommendation[] = job.recommendations ?? []

  // Build mock time-series from summary stats for visualisation
  // (real implementation would fetch metric_snapshots from the backend)
  const cpuData = [
    { name: 'Avg', value: s.cpu_percent_avg },
    { name: 'p95', value: s.cpu_percent_p95 },
    { name: 'Peak', value: s.cpu_percent_peak },
  ]
  const memData = [
    { name: 'Avg', value: Number(s.mem_used_gib_avg?.toFixed(2)) },
    { name: 'p95', value: Number(s.mem_used_gib_p95?.toFixed(2)) },
    { name: 'Peak', value: Number(s.mem_used_gib_peak?.toFixed(2)) },
  ]

  const costData = recs.slice(0, 6).map((r) => ({
    name: r.machine.id,
    monthly: Number(r.estimated_monthly_usd.toFixed(2)),
    current: Number(r.current_monthly_usd.toFixed(2)),
  }))

  return (
    <div className="fadein">
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8, marginBottom: 16 }}>
        <button
          onClick={() => navigate('/app')}
          style={{ background: 'none', border: 'none', color: '#9A7B5A', cursor: 'pointer', fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 13, letterSpacing: 1, padding: 0, whiteSpace: 'nowrap' }}
        >
          Jobs
        </button>
        <span aria-hidden="true" style={{ color: '#D4B896' }}>/</span>
        <Link
          to={`/app/jobs/group/${encodeURIComponent(job.job_id)}`}
          style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 13, letterSpacing: 1, color: '#9A7B5A', textDecoration: 'none', whiteSpace: 'nowrap' }}
        >
          {job.job_id}
        </Link>
        <span aria-hidden="true" style={{ color: '#D4B896' }}>/</span>
        <span style={{ fontFamily: "'Bebas Neue', Impact, sans-serif", fontSize: 13, letterSpacing: 1, color: '#2C1A0E', whiteSpace: 'nowrap' }}>Run #{job.id}</span>
      </div>
      <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] mb-5 tracking-tight">{job.job_id}</h1>

      {/* Peak stat cards */}
      <div className="stats-row">
        <div className="stat-card">
          <div className="stat-label">CPU p95</div>
          <div className="stat-value">{s.cpu_percent_p95?.toFixed(1)}<span className="stat-unit">%</span></div>
        </div>
        <div className="stat-card">
          <div className="stat-label">CPU Peak</div>
          <div className="stat-value">{s.cpu_percent_peak?.toFixed(1)}<span className="stat-unit">%</span></div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Mem p95</div>
          <div className="stat-value">{s.mem_used_gib_p95?.toFixed(2)}<span className="stat-unit">GiB</span></div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Mem Peak</div>
          <div className="stat-value">{s.mem_used_gib_peak?.toFixed(2)}<span className="stat-unit">GiB</span></div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Processes</div>
          <div className="stat-value">{s.process_count_peak}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Threads</div>
          <div className="stat-value">{s.thread_count_peak}</div>
        </div>
        <div className="stat-card">
          <div className="stat-label">Duration</div>
          <div className="stat-value">{s.duration_seconds?.toFixed(0)}<span className="stat-unit">s</span></div>
        </div>
        {s.detected_machine && (
          <div className="stat-card">
            <div className="stat-label">Detected</div>
            <div className="stat-value" style={{ fontSize: 14 }}>{s.detected_machine.id}</div>
            <div className="text-xs text-[var(--text-light)] mt-1">
              {s.detected_machine.storage_type || 'unknown storage'}
              {s.runtime_storage_class && s.runtime_storage_class !== 'unknown' ? ` · runtime ${s.runtime_storage_class}` : ''}
            </div>
            {(s.detected_machine_confidence_level || s.detected_machine_confidence != null) && (
              <div className="mt-1.5">
                <span className="badge" style={detectionTone(s.detected_machine_confidence_level)} title={s.detected_machine_match_reason || undefined}>
                  detection {s.detected_machine_confidence_level || 'unknown'}
                  {s.detected_machine_confidence != null ? ` (${Math.round(s.detected_machine_confidence * 100)}%)` : ''}
                </span>
              </div>
            )}
          </div>
        )}
        {s.ci_platform && (
          <div className="stat-card">
            <div className="stat-label">CI Platform</div>
            <div className="stat-value" style={{ fontSize: 14 }}>
              <span className={`badge badge-ci-${s.ci_platform}`}>
                {s.ci_platform === 'github' ? 'GitHub Actions'
                  : s.ci_platform === 'gitlab' ? 'GitLab CI'
                  : s.ci_platform === 'circleci' ? 'CircleCI'
                  : s.ci_platform === 'bitbucket' ? 'Bitbucket'
                  : s.ci_platform === 'azure' ? 'Azure Pipelines'
                  : s.ci_platform === 'jenkins' ? 'Jenkins'
                  : s.ci_platform}
              </span>
            </div>
          </div>
        )}
      </div>

      {/* CPU chart */}
      <div className="card">
        <h2>CPU Usage</h2>
        <div className="chart-wrap">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={cpuData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#D4B896" />
              <XAxis dataKey="name" stroke="#9A7B5A" tick={{ fontFamily: "'Lato'", fontSize: 12 }} />
              <YAxis domain={[0, 100]} stroke="#9A7B5A" unit="%" tick={{ fontFamily: "'Lato'", fontSize: 11 }} />
              <Tooltip contentStyle={{ background: '#FFFDF7', border: '1px solid #D4B896', fontFamily: 'Lato', color: '#2C1A0E' }} />
              <Bar dataKey="value" fill="#C23B22" radius={[3, 3, 0, 0]} name="CPU %" />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </div>

      {/* Memory chart */}
      <div className="card">
        <h2>Memory Usage</h2>
        <div className="chart-wrap">
          <ResponsiveContainer width="100%" height="100%">
            <BarChart data={memData}>
              <CartesianGrid strokeDasharray="3 3" stroke="#D4B896" />
              <XAxis dataKey="name" stroke="#9A7B5A" tick={{ fontFamily: "'Lato'", fontSize: 12 }} />
              <YAxis stroke="#9A7B5A" unit=" GiB" tick={{ fontFamily: "'Lato'", fontSize: 11 }} />
              <Tooltip contentStyle={{ background: '#FFFDF7', border: '1px solid #D4B896', fontFamily: 'Lato', color: '#2C1A0E' }} />
              <Bar dataKey="value" fill="#1B3361" radius={[3, 3, 0, 0]} name="Memory GiB" />
            </BarChart>
          </ResponsiveContainer>
        </div>
      </div>

      {/* Cost comparison */}
      {costData.length > 0 && (
        <div className="card">
          <h2>Cost Comparison (monthly)</h2>
          <div className="chart-wrap">
            <ResponsiveContainer width="100%" height="100%">
              <BarChart data={costData} layout="vertical">
                <CartesianGrid strokeDasharray="3 3" stroke="#D4B896" />
                <XAxis
                  type="number"
                  stroke="#9A7B5A"
                  tick={{ fontFamily: "'Lato'", fontSize: 11 }}
                  tickFormatter={(value) => formatFromUSD(Number(value), currency, { minimumFractionDigits: 0, maximumFractionDigits: 0 })}
                />
                <YAxis type="category" dataKey="name" stroke="#9A7B5A" width={120} tick={{ fontSize: 11, fontFamily: 'monospace' }} />
                <Tooltip
                  contentStyle={{ background: '#FFFDF7', border: '1px solid #D4B896', fontFamily: 'Lato', color: '#2C1A0E' }}
                  formatter={(value) => formatFromUSD(Number(value ?? 0), currency)}
                />
                <Legend wrapperStyle={{ fontFamily: "'Lato'", fontSize: 13 }} />
                <Bar dataKey="current" fill="#D4B896" name="Current" radius={[0, 3, 3, 0]} />
                <Bar dataKey="monthly" fill="#B8860B" name="Recommended" radius={[0, 3, 3, 0]} />
              </BarChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}

      {/* Recommendations table */}
      <div className="card">
        <h2>Recommendations</h2>
        {recs.length === 0 ? (
          <div className="empty">No recommendations available.</div>
        ) : (
          <>
            {recs[0]?.duration_regression_pct != null && (
              <div style={{ background: '#FFF3CD', border: '1px solid #F5C842', borderRadius: 4, padding: '8px 14px', marginBottom: 12, fontFamily: 'Lato, sans-serif', fontSize: 13, color: '#7B5800' }}>
                Duration regression detected: this run was <strong>+{recs[0].duration_regression_pct.toFixed(1)}%</strong> slower than the rolling average.
              </div>
            )}
            <div className="table-wrap">
              <table className="rr-table">
                <thead>
                  <tr>
                    <th>Tier</th>
                    <th>Machine</th>
                    <th>Provider</th>
                    <th>vCPUs</th>
                    <th>Memory</th>
                    <th>Arch</th>
                    <th>Price/hr</th>
                    <th>Price/month</th>
                    <th title="Approximate spot/preemptible price">Spot/mo</th>
                    <th>Delta</th>
                  </tr>
                </thead>
                <tbody>
                  {recs.map((r, i) => (
                    <tr key={i}>
                      <td><TierBadge tier={r.tier} /></td>
                      <td><code>{r.machine.id}</code></td>
                      <td><span className={`badge badge-${r.machine.provider}`}>{r.machine.provider.toUpperCase()}</span></td>
                      <td>{r.machine.vcpus}</td>
                      <td>{r.machine.memory_gib} GiB</td>
                      <td>{r.machine.architecture}</td>
                      <td>{formatFromUSD(r.machine.on_demand_price_per_hour, currency, { minimumFractionDigits: 4, maximumFractionDigits: 4 })}</td>
                      <td>{formatFromUSD(r.estimated_monthly_usd, currency)}</td>
                      <td style={{ color: '#5A8A3A', fontFamily: 'monospace', fontSize: 13 }}>
                        {(r.spot_monthly_usd ?? 0) > 0 ? formatFromUSD(r.spot_monthly_usd ?? 0, currency) : '—'}
                      </td>
                      <td><DeltaCell pct={r.cost_delta_percent} /></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}
      </div>
    </div>
  )
}
