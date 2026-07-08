import { useNavigate } from 'react-router-dom'
import { formatFromUSD } from '../../currency'
import type { CurrencyCode } from '../../currency'
import type { SortKey, JobGroup } from './types'
import { ciLabel, deltaClass, formatDelta, benefitStyle, spotRiskStyle } from './utils'

interface SortIndicatorProps {
  col: SortKey
  sortKey: SortKey
  sortDir: 'asc' | 'desc'
}

function SortIndicator({ col, sortKey, sortDir }: SortIndicatorProps) {
  if (sortKey !== col) return <span style={{ color: '#D4B896', marginLeft: 3 }}>⇅</span>
  return <span style={{ marginLeft: 3, color: '#C23B22' }}>{sortDir === 'asc' ? '↑' : '↓'}</span>
}

interface JobsTableProps {
  jobs: JobGroup[]
  sortKey: SortKey
  sortDir: 'asc' | 'desc'
  currency: CurrencyCode
  onSort: (key: SortKey) => void
}

export default function JobsTable({ jobs, sortKey, sortDir, currency, onSort }: JobsTableProps) {
  const navigate = useNavigate()

  const goToJob = (jobId: string) => navigate(`/app/jobs/group/${encodeURIComponent(jobId)}`)

  return (
    <>
      {/* Mobile cards */}
      <div className="sm:hidden grid gap-3 mb-4">
        {jobs.map((g) => (
          <button
            key={g.jobId}
            onClick={() => goToJob(g.jobId)}
            className="text-left bg-paper border border-[var(--border)] rounded-lg px-4 py-3 shadow-rr"
          >
            <div className="flex items-start justify-between gap-3 mb-2">
              <div>
                <div className="font-mono text-[13px] text-[var(--text)] break-all">{g.jobId}</div>
                <div className="text-xs text-[var(--text-light)] mt-1 break-all">
                  {g.repository || 'No repository'}
                </div>
              </div>
              <span className={`badge badge-${g.tier}`}>{g.tier}</span>
            </div>
            <div className="grid grid-cols-2 gap-2 text-xs text-[var(--text-mid)]">
              <div>
                <span className="text-[var(--text-light)]">Savings</span>
                <div className={deltaClass(g.avgCostDelta)}>{formatDelta(g.avgCostDelta)}</div>
              </div>
              <div>
                <span className="text-[var(--text-light)]">Benefit</span>
                <div>
                  <span className="badge" style={benefitStyle(g.benefitTone)}>
                    {g.benefitLabel}
                  </span>
                </div>
              </div>
              <div>
                <span className="text-[var(--text-light)]">Runs</span>
                <div>{g.runCount}</div>
              </div>
              <div>
                <span className="text-[var(--text-light)]">CPU</span>
                <div>{g.medianCpu.toFixed(1)}%</div>
              </div>
              <div>
                <span className="text-[var(--text-light)]">Mem</span>
                <div>{g.medianMem.toFixed(2)} GiB</div>
              </div>
              <div>
                <span className="text-[var(--text-light)]">Spot</span>
                <div>
                  {g.spotRisk ? (
                    <span className="badge" style={spotRiskStyle(g.spotRisk)}>
                      {g.spotRisk}
                    </span>
                  ) : (
                    '—'
                  )}
                </div>
              </div>
              <div className="col-span-2">
                <span className="text-[var(--text-light)]">Running On</span>
                <div className="font-mono text-[12px] break-all">{g.detectedMachine}</div>
              </div>
              <div className="col-span-2">
                <span className="text-[var(--text-light)]">Best Fit</span>
                <div className="font-mono text-[12px] break-all">{g.suggestedMachine}</div>
              </div>
            </div>
          </button>
        ))}
      </div>

      {/* Desktop table */}
      <div className="rr-card !mb-2 !p-0 hidden sm:block">
        <div className="table-wrap">
          <table className="rr-table">
            <thead>
              <tr>
                <th>Job Name</th>
                <th className="hidden sm:table-cell">Repository</th>
                <th className="hidden md:table-cell">CI</th>
                <th className="hidden md:table-cell">Provider</th>
                <th className="hidden lg:table-cell">Running On</th>
                <th className="hidden lg:table-cell">Best Fit</th>
                <th>Tier</th>
                <th
                  className="cursor-pointer select-none"
                  onClick={() => onSort('avgCostDelta')}
                  title="Average cost delta across all runs"
                >
                  Avg Savings <SortIndicator col="avgCostDelta" sortKey={sortKey} sortDir={sortDir} />
                </th>
                <th className="hidden lg:table-cell">Benefit</th>
                <th className="hidden lg:table-cell">Spot</th>
                <th
                  className="cursor-pointer select-none hidden xl:table-cell"
                  onClick={() => onSort('medianCpu')}
                  title="Median p95 CPU"
                >
                  CPU <SortIndicator col="medianCpu" sortKey={sortKey} sortDir={sortDir} />
                </th>
                <th
                  className="cursor-pointer select-none hidden xl:table-cell"
                  onClick={() => onSort('medianMem')}
                  title="Median p95 memory"
                >
                  Mem <SortIndicator col="medianMem" sortKey={sortKey} sortDir={sortDir} />
                </th>
                <th className="cursor-pointer select-none hidden lg:table-cell" onClick={() => onSort('medianDuration')}>
                  Dur <SortIndicator col="medianDuration" sortKey={sortKey} sortDir={sortDir} />
                </th>
                <th className="cursor-pointer select-none hidden sm:table-cell" onClick={() => onSort('runCount')}>
                  Runs <SortIndicator col="runCount" sortKey={sortKey} sortDir={sortDir} />
                </th>
                <th className="cursor-pointer select-none" onClick={() => onSort('latestDate')}>
                  Last Run <SortIndicator col="latestDate" sortKey={sortKey} sortDir={sortDir} />
                </th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((g) => (
                <tr key={g.jobId} className="cursor-pointer" onClick={() => goToJob(g.jobId)}>
                  <td className="td-job-name" title={g.jobId}>
                    <button className="link-btn font-mono text-[13px]">{g.jobId}</button>
                  </td>
                  <td
                    className="hidden sm:table-cell whitespace-nowrap font-mono text-xs text-[var(--text-light)]"
                    title={g.repository || undefined}
                  >
                    {g.repository ? (
                      <a
                        href={`https://github.com/${g.repository}`}
                        target="_blank"
                        rel="noreferrer"
                        className="text-[var(--text-light)] no-underline hover:text-[var(--red)]"
                        onClick={(e) => e.stopPropagation()}
                      >
                        {g.repository}
                      </a>
                    ) : (
                      <span className="text-[var(--border)]">—</span>
                    )}
                  </td>
                  <td className="hidden md:table-cell">
                    {g.ciPlatform && (
                      <span className={`badge badge-ci-${g.ciPlatform}`}>{ciLabel(g.ciPlatform)}</span>
                    )}
                  </td>
                  <td className="hidden md:table-cell">
                    {g.provider && (
                      <span className={`badge badge-${g.provider}`}>{g.provider.toUpperCase()}</span>
                    )}
                  </td>
                  <td className="hidden lg:table-cell font-mono text-[13px]">{g.detectedMachine}</td>
                  <td className="hidden lg:table-cell font-mono text-[13px]">{g.suggestedMachine}</td>
                  <td>{g.tier ? <span className={`badge badge-${g.tier}`}>{g.tier}</span> : null}</td>
                  <td>
                    <span className={deltaClass(g.avgCostDelta)}>{formatDelta(g.avgCostDelta)}</span>
                  </td>
                  <td className="hidden lg:table-cell">
                    <span className="badge" style={benefitStyle(g.benefitTone)}>
                      {g.benefitLabel}
                    </span>
                  </td>
                  <td className="hidden lg:table-cell">
                    {g.spotRisk ? (
                      <span
                        className="badge"
                        style={spotRiskStyle(g.spotRisk)}
                        title={g.avgSpotDelta ? `avg spot delta ${formatDelta(g.avgSpotDelta)}` : undefined}
                      >
                        {g.spotRisk}
                      </span>
                    ) : (
                      <span className="text-[var(--border)]">—</span>
                    )}
                  </td>
                  <td className="hidden xl:table-cell">{g.medianCpu.toFixed(1)}%</td>
                  <td className="hidden xl:table-cell">{g.medianMem.toFixed(2)} GiB</td>
                  <td className="hidden lg:table-cell">{g.medianDuration.toFixed(0)}s</td>
                  <td className="hidden sm:table-cell">{g.runCount}</td>
                  <td>{new Date(g.latestDate).toLocaleDateString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <p className="text-xs text-[var(--text-light)] mb-4 font-sans">
        All metrics are medians across every run of that job. <strong>CPU</strong> and <strong>Memory</strong> use the p95
        value from each run, then take the median of those across runs. Click a row to see the full run history.
      </p>
    </>
  )
}
