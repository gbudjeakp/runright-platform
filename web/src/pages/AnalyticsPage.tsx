import { useState, useEffect, type ReactNode } from 'react'
import { useCurrencyPreference, formatFromUSD } from '../currency'
import { DateRangePicker, EMPTY_RANGE, type DateRange } from '../components/DateRangePicker'

// Types matching backend
interface RepoStats {
  repository: string
  job_count: number
  total_cost: number
  potential_savings: number
}

interface TrendPoint {
  date: string
  value: number
}

interface ResourceUtilization {
  avg_cpu: number
  avg_memory: number
  avg_disk: number
  idle_percent: number
}

interface AnalyticsSummary {
  total_jobs: number
  total_cost: number
  total_savings: number
  savings_percent: number
  avg_cost_per_job: number
  top_repositories: RepoStats[]
  cost_trend: TrendPoint[]
  savings_trend: TrendPoint[]
  resource_utilization: ResourceUtilization
}

interface CostBreakdown {
  by_provider: { name: string; cost: number; percent: number }[]
  by_repo: { name: string; cost: number; percent: number }[]
  by_tier: { name: string; cost: number; percent: number }[]
}

interface CarbonMetrics {
  total_kg_co2: number
  saved_kg_co2: number
  equivalent_tree_days: number
  equivalent_car_miles: number
}

type Period = '7d' | '30d' | '90d' | '1y'

function rangeToQuery(range: DateRange): string {
  if (!range.from && !range.to) return 'period=all'
  if (range.from && range.to) return `from=${range.from}&to=${range.to}`
  if (range.from) return `from=${range.from}`
  return 'period=30d'
}

export default function AnalyticsPage() {
  const { currency } = useCurrencyPreference()
  const format = (v: number) => formatFromUSD(v, currency)
  const [dateRange, setDateRange] = useState<DateRange>(EMPTY_RANGE)
  const [summary, setSummary] = useState<AnalyticsSummary | null>(null)
  const [breakdown, setBreakdown] = useState<CostBreakdown | null>(null)
  const [carbon, setCarbon] = useState<CarbonMetrics | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    loadData()
  }, [dateRange])

  async function loadData() {
    setLoading(true)
    setError('')
    const q = rangeToQuery(dateRange)
    try {
      const [summaryRes, breakdownRes, carbonRes] = await Promise.all([
        fetch(`/api/v1/analytics/summary?${q}`, { credentials: 'include' }),
        fetch(`/api/v1/analytics/cost-breakdown?${q}`, { credentials: 'include' }),
        fetch(`/api/v1/analytics/carbon?${q}`, { credentials: 'include' }),
      ])
      if (summaryRes.ok) setSummary(await summaryRes.json())
      if (breakdownRes.ok) setBreakdown(await breakdownRes.json())
      if (carbonRes.ok) setCarbon(await carbonRes.json())
    } catch {
      setError('Failed to load analytics')
    } finally {
      setLoading(false)
    }
  }

  if (loading) {
    return (
      <div className="fadein max-w-7xl">
        <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] mb-4 tracking-tight">
          Usage Analytics
        </h1>
        <div className="mb-6">
          <DateRangePicker value={dateRange} onChange={(r) => setDateRange(r)} />
        </div>
        <div className="text-[var(--text-light)] py-12 text-center">Loading analytics...</div>
      </div>
    )
  }

  return (
    <div className="fadein max-w-7xl">
      <div className="flex flex-wrap items-center justify-between gap-4 mb-6">
        <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] tracking-tight">
          Usage Analytics
        </h1>
        <DateRangePicker value={dateRange} onChange={(r) => { setDateRange(r) }} />
      </div>

      {error && (
        <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded mb-6">
          {error}
        </div>
      )}

      {/* Summary Stats */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
        <StatCard
          label="Total Jobs"
          value={summary?.total_jobs?.toLocaleString() ?? '0'}
          icon={<JobsIcon />}
        />
        <StatCard
          label="Total Cost"
          value={format(summary?.total_cost ?? 0)}
          icon={<CostIcon />}
        />
        <StatCard
          label="Potential Savings"
          value={format(summary?.total_savings ?? 0)}
          subtext={`${(summary?.savings_percent ?? 0).toFixed(1)}% of spend`}
          icon={<SavingsIcon />}
        />
        <StatCard
          label="Avg Cost/Job"
          value={format(summary?.avg_cost_per_job ?? 0)}
          icon={<AvgIcon />}
        />
      </div>

      {/* Charts Row */}
      <div className="grid lg:grid-cols-2 gap-6 mb-8">
        {/* Cost Trend */}
        <Card title="Cost Trend">
          {summary?.cost_trend?.length ? (
            <TrendLineChart
              data={summary.cost_trend}
              formatValue={format}
              color="var(--text-mid)"
              fillColor="var(--text-mid)"
            />
          ) : (
            <EmptyChart message="No cost data available" />
          )}
        </Card>

        {/* Savings Trend */}
        <Card title="Savings Trend">
          {summary?.savings_trend?.length ? (
            <TrendLineChart
              data={summary.savings_trend}
              formatValue={format}
              color="var(--red)"
              fillColor="var(--red)"
            />
          ) : (
            <EmptyChart message="No savings data available" />
          )}
        </Card>
      </div>

      {/* Resource Utilization */}
      <div className="grid lg:grid-cols-3 gap-6 mb-8">
        <Card title="Resource Utilization" className="lg:col-span-1">
          <div className="space-y-4">
            <UtilBar label="CPU" value={summary?.resource_utilization?.avg_cpu ?? 0} />
            <UtilBar label="Memory" value={summary?.resource_utilization?.avg_memory ?? 0} />
            <UtilBar label="Disk" value={summary?.resource_utilization?.avg_disk ?? 0} />
            <div className="pt-2 border-t border-[var(--border)]">
              <div className="flex justify-between text-sm">
                <span className="text-[var(--text-light)]">Idle Time</span>
                <span className="font-medium text-[var(--text)]">
                  {(summary?.resource_utilization?.idle_percent ?? 0).toFixed(1)}%
                </span>
              </div>
            </div>
          </div>
        </Card>

        {/* Cost Breakdown by Provider */}
        <Card title="Cost by Provider" className="lg:col-span-2">
          {breakdown?.by_provider?.length ? (
            <BreakdownTable data={breakdown.by_provider} formatValue={format} />
          ) : (
            <div className="text-sm text-[var(--text-light)] py-4 text-center">No data available</div>
          )}
        </Card>
      </div>

      {/* Top Repositories */}
      <Card title="Top Repositories by Cost">
        {summary?.top_repositories?.length ? (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="text-left text-[var(--text-light)] border-b border-[var(--border)]">
                  <th className="pb-3 font-medium">Repository</th>
                  <th className="pb-3 font-medium text-right">Jobs</th>
                  <th className="pb-3 font-medium text-right">Cost</th>
                  <th className="pb-3 font-medium text-right">Potential Savings</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[var(--border)]">
                {summary.top_repositories.map((repo, i) => (
                  <tr key={repo.repository || i} className="hover:bg-[var(--cream-alt)]">
                    <td className="py-3 font-medium text-[var(--text)] truncate max-w-[300px]">
                      {repo.repository || 'Unknown'}
                    </td>
                    <td className="py-3 text-right text-[var(--text-mid)]">
                      {repo.job_count.toLocaleString()}
                    </td>
                    <td className="py-3 text-right text-[var(--text)]">
                      {format(repo.total_cost)}
                    </td>
                    <td className="py-3 text-right font-medium text-[var(--text)]">
                      {repo.potential_savings > 0 ? format(repo.potential_savings) : '$0.00'}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="text-sm text-[var(--text-light)] py-8 text-center">
            No repository data available
          </div>
        )}
      </Card>

      {/* Carbon Footprint */}
      <Card title="Carbon Footprint" className="mt-8">
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          <CarbonStat
            label="Total CO₂"
            value={`${(carbon?.total_kg_co2 ?? 0).toFixed(2)} kg`}
            icon={<CloudIcon />}
          />
          <CarbonStat
            label="CO₂ Saveable"
            value={`${(carbon?.saved_kg_co2 ?? 0).toFixed(2)} kg`}
            icon={<LeafIcon />}
            highlight
          />
          <CarbonStat
            label="Tree Days Equivalent"
            value={`${(carbon?.equivalent_tree_days ?? 0).toFixed(0)} days`}
            icon={<TreeIcon />}
            subtext="of carbon absorption"
          />
          <CarbonStat
            label="Car Miles Equivalent"
            value={`${(carbon?.equivalent_car_miles ?? 0).toFixed(0)} mi`}
            icon={<CarIcon />}
            subtext="of driving avoided"
          />
        </div>
        <div className="mt-6 text-xs text-[var(--text-light)] bg-[var(--cream-alt)] p-3 rounded-lg">
          <strong>How we calculate:</strong> Carbon estimates use EPA emission factors per cloud provider
          (AWS: 0.417, GCP: 0.180, Azure: 0.350 kg CO₂/kWh) and approximate power consumption based on vCPUs and memory.
          Savings represent potential reductions from right-sizing to recommended machines.
        </div>
      </Card>
    </div>
  )
}

// === Components ===

function Card({ title, children, className = '' }: { title: string; children: ReactNode; className?: string }) {
  return (
    <div className={`bg-paper border border-[var(--border)] rounded-lg shadow-sm ${className}`}>
      <div className="px-5 py-4 border-b border-[var(--border)]">
        <h2 className="font-semibold text-[var(--text)]">{title}</h2>
      </div>
      <div className="px-5 py-5">{children}</div>
    </div>
  )
}

function StatCard({
  label,
  value,
  subtext,
  icon,
  highlight = false,
}: {
  label: string
  value: string
  subtext?: string
  icon: ReactNode
  highlight?: boolean
}) {
  return (
    <div className={`bg-paper border rounded-lg p-5 ${highlight ? 'border-[var(--red)] ring-2 ring-[var(--red)] ring-opacity-30 bg-red-50 dark:bg-red-950/20' : 'border-[var(--border)]'}`}>
      <div className="flex items-start justify-between mb-3">
        <span className={`text-sm ${highlight ? 'text-[var(--red)] font-medium' : 'text-[var(--text-light)]'}`}>{label}</span>
        <span className={`w-5 h-5 ${highlight ? 'text-[var(--red)]' : 'text-[var(--text-light)]'}`}>{icon}</span>
      </div>
      <div className={`text-2xl font-bold ${highlight ? 'text-[var(--red)]' : 'text-[var(--text)]'}`}>
        {value}
      </div>
      {subtext && <div className="text-xs text-[var(--text-light)] mt-1">{subtext}</div>}
    </div>
  )
}

function UtilBar({ label, value }: { label: string; value: number }) {
  const pct = Math.min(100, Math.max(0, value))
  // Use brown/amber color scheme matching the app theme
  const color = pct > 80 ? 'bg-[var(--red)]' : pct > 50 ? 'bg-amber-600' : 'bg-[var(--text-mid)]'
  return (
    <div>
      <div className="flex justify-between text-sm mb-1.5">
        <span className="text-[var(--text-light)]">{label}</span>
        <span className="font-medium text-[var(--text)]">{pct.toFixed(1)}%</span>
      </div>
      <div className="h-2 bg-[var(--border)] rounded-full overflow-hidden">
        <div className={`h-full ${color} rounded-full transition-all`} style={{ width: `${pct}%` }} />
      </div>
    </div>
  )
}

function TrendLineChart({
  data,
  formatValue,
  color,
  fillColor,
}: {
  data: TrendPoint[]
  formatValue: (v: number) => string
  color: string
  fillColor: string
}) {
  const width = 100
  const height = 48
  const padding = { top: 4, right: 2, bottom: 16, left: 2 }
  const chartW = width - padding.left - padding.right
  const chartH = height - padding.top - padding.bottom

  const maxValue = Math.max(...data.map((d) => d.value), 1)
  const minValue = Math.min(...data.map((d) => d.value))
  const range = maxValue - minValue || 1

  const points = data.map((d, i) => ({
    x: padding.left + (i / (data.length - 1 || 1)) * chartW,
    y: padding.top + chartH - ((d.value - minValue) / range) * chartH,
    value: d.value,
    date: d.date,
  }))

  const linePath = points.map((p, i) => `${i === 0 ? 'M' : 'L'} ${p.x} ${p.y}`).join(' ')
  const areaPath = `${linePath} L ${points[points.length - 1].x} ${padding.top + chartH} L ${points[0].x} ${padding.top + chartH} Z`

  // Show ~5 evenly spaced labels
  const labelInterval = Math.ceil(data.length / 5)

  return (
    <div className="h-52 relative">
      <svg viewBox={`0 0 ${width} ${height}`} className="w-full h-44" preserveAspectRatio="none">
        <defs>
          <linearGradient id={`gradient-${color.replace(/[^a-z0-9]/gi, '')}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={fillColor} stopOpacity="0.3" />
            <stop offset="100%" stopColor={fillColor} stopOpacity="0.05" />
          </linearGradient>
        </defs>
        {/* Area fill */}
        <path d={areaPath} fill={`url(#gradient-${color.replace(/[^a-z0-9]/gi, '')})`} />
        {/* Line */}
        <path d={linePath} fill="none" stroke={color} strokeWidth="0.4" strokeLinecap="round" strokeLinejoin="round" />
        {/* Points */}
        {points.map((p, i) => (
          <circle key={i} cx={p.x} cy={p.y} r="0.6" fill={color} className="hover:r-[1] cursor-pointer" />
        ))}
      </svg>
      {/* X-axis labels */}
      <div className="flex justify-between text-[10px] text-[var(--text-light)] px-1 -mt-1">
        {data.map((d, i) =>
          i % labelInterval === 0 || i === data.length - 1 ? (
            <span key={d.date} className="truncate">
              {d.date.slice(5)}
            </span>
          ) : (
            <span key={d.date} />
          )
        )}
      </div>
      {/* Hover tooltips via overlay divs */}
      <div className="absolute inset-0 flex" style={{ top: 0, height: '176px' }}>
        {points.map((p, i) => (
          <div key={i} className="flex-1 group relative">
            <div className="absolute left-1/2 -translate-x-1/2 bg-[var(--text)] text-[var(--cream)] text-xs px-2 py-1 rounded opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap pointer-events-none z-10" style={{ top: `${(p.y / height) * 100}%` }}>
              {formatValue(p.value)}
              <div className="text-[10px] opacity-70">{p.date}</div>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function EmptyChart({ message }: { message: string }) {
  return (
    <div className="h-48 flex items-center justify-center text-sm text-[var(--text-light)]">
      {message}
    </div>
  )
}

function BreakdownTable({
  data,
  formatValue,
}: {
  data: { name: string; cost: number; percent: number }[]
  formatValue: (v: number) => string
}) {
  return (
    <div className="space-y-3">
      {data.map((item, i) => (
        <div key={item.name || i}>
          <div className="flex justify-between text-sm mb-1">
            <span className="text-[var(--text)] font-medium">{item.name || 'Unknown'}</span>
            <span className="text-[var(--text-mid)]">
              {formatValue(item.cost)} ({item.percent.toFixed(1)}%)
            </span>
          </div>
          <div className="h-1.5 bg-[var(--border)] rounded-full overflow-hidden">
            <div
              className="h-full bg-[var(--text-mid)] rounded-full"
              style={{ width: `${item.percent}%` }}
            />
          </div>
        </div>
      ))}
    </div>
  )
}

// Icons
function JobsIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="3" y="3" width="18" height="18" rx="2" />
      <path d="M9 9h6M9 15h6" />
    </svg>
  )
}

function CostIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <line x1="12" y1="1" x2="12" y2="23" />
      <path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6" />
    </svg>
  )
}

function SavingsIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="23 18 13.5 8.5 8.5 13.5 1 6" />
      <polyline points="17 18 23 18 23 12" />
    </svg>
  )
}

function AvgIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M3 3v18h18" />
      <path d="M18 17V9" />
      <path d="M13 17V5" />
      <path d="M8 17v-3" />
    </svg>
  )
}

// Carbon-related components and icons
function CarbonStat({
  label,
  value,
  subtext,
  icon,
  highlight = false,
}: {
  label: string
  value: string
  subtext?: string
  icon: ReactNode
  highlight?: boolean
}) {
  return (
    <div className={`text-center p-4 rounded-lg ${highlight ? 'bg-green-50 dark:bg-green-950/20 border border-green-200' : 'bg-[var(--cream-alt)]'}`}>
      <div className={`w-8 h-8 mx-auto mb-2 ${highlight ? 'text-green-600' : 'text-[var(--text-light)]'}`}>
        {icon}
      </div>
      <div className={`text-xl font-bold ${highlight ? 'text-green-700' : 'text-[var(--text)]'}`}>
        {value}
      </div>
      <div className="text-xs text-[var(--text-light)] mt-1">{label}</div>
      {subtext && <div className="text-[10px] text-[var(--text-light)] opacity-70">{subtext}</div>}
    </div>
  )
}

function CloudIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M18 10h-1.26A8 8 0 1 0 9 20h9a5 5 0 0 0 0-10z" />
    </svg>
  )
}

function LeafIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M11 20A7 7 0 0 1 9.8 6.1C15.5 5 17 4.48 19 2c1 2 2 4.18 2 8 0 5.5-4.78 10-10 10Z" />
      <path d="M2 21c0-3 1.85-5.36 5.08-6C9.5 14.52 12 13 13 12" />
    </svg>
  )
}

function TreeIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 22v-7" />
      <path d="m17 8-5-6-5 6h4v4H7l5 6 5-6h-4V8h4Z" />
    </svg>
  )
}

function CarIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M19 17h2c.6 0 1-.4 1-1v-3c0-.9-.7-1.7-1.5-1.9C18.7 10.6 16 10 16 10s-1.3-1.4-2.2-2.3c-.5-.4-1.1-.7-1.8-.7H5c-.6 0-1.1.4-1.4.9l-1.4 2.9A3.7 3.7 0 0 0 2 12v4c0 .6.4 1 1 1h2" />
      <circle cx="7" cy="17" r="2" />
      <circle cx="17" cy="17" r="2" />
    </svg>
  )
}
