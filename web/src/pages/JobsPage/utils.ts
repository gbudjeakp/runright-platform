import type { Job } from '../../types'
import { inDateRange } from '../../components/DateRangePicker'
import type { DateRange } from '../../components/DateRangePicker'
import type { JobGroup } from './types'

export function round2(n: number): number {
  return Math.round(n * 100) / 100
}

export function formatShortDate(input: string): string {
  const d = new Date(input)
  if (Number.isNaN(d.getTime())) return input.slice(0, 10)
  return d.toLocaleDateString(undefined, { month: '2-digit', day: '2-digit' })
}

export function median(values: number[]): number {
  if (values.length === 0) return 0
  const sorted = [...values].sort((a, b) => a - b)
  const mid = Math.floor(sorted.length / 2)
  return sorted.length % 2 === 0 ? (sorted[mid - 1] + sorted[mid]) / 2 : sorted[mid]
}

export function mode<T>(values: T[]): T | undefined {
  if (values.length === 0) return undefined
  const counts = new Map<string, { val: T; count: number }>()
  for (const v of values) {
    const key = String(v)
    const entry = counts.get(key)
    if (entry) entry.count++
    else counts.set(key, { val: v, count: 1 })
  }
  return [...counts.values()].sort((a, b) => b.count - a.count)[0].val
}

export function deriveBenefit(
  avgCostDelta: number,
  tier: string
): { label: string; tone: 'saving' | 'benefit' | 'neutral' } {
  if (avgCostDelta < -0.5) return { label: 'Cost reduction', tone: 'saving' }
  if (tier === 'more-headroom' || avgCostDelta > 0.5) return { label: 'Performance / stability', tone: 'benefit' }
  return { label: 'Right-sized fit', tone: 'neutral' }
}

export function deltaClass(pct: number) {
  if (pct < -0.5) return 'delta-negative'
  if (pct > 0.5) return 'delta-positive'
  return 'delta-neutral'
}

export function formatDelta(pct: number) {
  return `${pct > 0 ? '+' : ''}${pct.toFixed(1)}%`
}

export function spotRiskStyle(risk: string): React.CSSProperties {
  switch (risk) {
    case 'low':
      return { color: '#2E7D32', borderColor: '#2E7D32', background: 'rgba(46,125,50,.08)' }
    case 'high':
      return { color: '#C23B22', borderColor: '#C23B22', background: 'rgba(194,59,34,.08)' }
    default:
      return { color: '#9A7B5A', borderColor: '#9A7B5A', background: 'rgba(154,123,90,.08)' }
  }
}

export function benefitStyle(tone: 'saving' | 'benefit' | 'neutral'): React.CSSProperties {
  switch (tone) {
    case 'saving':
      return { color: '#2E7D32', borderColor: '#2E7D32', background: 'rgba(46,125,50,.08)' }
    case 'benefit':
      return { color: '#1B3361', borderColor: '#1B3361', background: 'rgba(27,51,97,.08)' }
    default:
      return { color: '#9A7B5A', borderColor: '#9A7B5A', background: 'rgba(154,123,90,.08)' }
  }
}

export const CI_LABELS: Record<string, string> = {
  github: 'GitHub Actions',
  gitlab: 'GitLab CI',
  circleci: 'CircleCI',
  bitbucket: 'Bitbucket',
  azure: 'Azure Pipelines',
  jenkins: 'Jenkins',
  local: 'Local',
}

export function ciLabel(platform: string) {
  return CI_LABELS[platform] ?? platform
}

export function groupJobs(jobs: Job[], dateRange: DateRange): JobGroup[] {
  const groups = new Map<string, Job[]>()
  for (const job of jobs) {
    const arr = groups.get(job.job_id) ?? []
    arr.push(job)
    groups.set(job.job_id, arr)
  }

  const periodDays =
    dateRange.from && dateRange.to
      ? (new Date(dateRange.to).getTime() - new Date(dateRange.from).getTime()) / (1000 * 60 * 60 * 24)
      : 90

  return Array.from(groups.entries()).flatMap(([jobId, runs]) => {
    const latestOverallDate = runs.map((r) => r.start_time).sort().at(-1) ?? ''
    if (!inDateRange(latestOverallDate, dateRange)) return []

    const runsInRange = runs.filter((r) => inDateRange(r.start_time, dateRange))
    if (runsInRange.length === 0) return []

    const cpu = runsInRange.map((r) => r.summary?.cpu_percent_p95 ?? 0).filter((v) => v > 0)
    const mem = runsInRange.map((r) => r.summary?.mem_used_gib_p95 ?? 0).filter((v) => v > 0)
    const dur = runsInRange.map((r) => r.duration_seconds ?? 0).filter((v) => v > 0)
    const deltas = runsInRange.map((r) => r.recommendations?.[0]?.cost_delta_percent ?? 0)
    const spotDeltas = runsInRange
      .map((r) => r.recommendations?.[0]?.spot_delta_percent)
      .filter((v): v is number => typeof v === 'number')
    const spotRisks = runsInRange.map((r) => r.recommendations?.[0]?.spot_risk ?? '').filter(Boolean)
    const providers = runsInRange.map((r) => r.summary?.detected_machine?.provider ?? '').filter(Boolean)
    const detected = runsInRange.map((r) => r.summary?.detected_machine?.id ?? '').filter(Boolean)
    const suggested = runsInRange.map((r) => r.recommendations?.[0]?.machine.id ?? '').filter(Boolean)
    const allTiers = runsInRange.flatMap((r) => r.recommendations?.map((rec) => rec.tier) ?? []).filter(Boolean)
    const availableTiers = [...new Set(allTiers)]
    const tier = availableTiers.includes('more-headroom')
      ? 'more-headroom'
      : availableTiers.includes('cheaper-option')
        ? 'cheaper-option'
        : availableTiers[0] ?? ''
    const platforms = runsInRange.map((r) => r.summary?.ci_platform ?? '').filter(Boolean)
    const ciPlatform = mode(platforms) ?? ''
    const repos = runsInRange.map((r) => r.repository ?? '').filter(Boolean)
    const repository = mode(repos) ?? ''

    const avgCostDelta = deltas.length ? deltas.reduce((a, b) => a + b, 0) / deltas.length : 0
    const avgSpotDelta = spotDeltas.length ? spotDeltas.reduce((a, b) => a + b, 0) / spotDeltas.length : 0
    const spotRisk = mode(spotRisks) ?? ''
    const benefit = deriveBenefit(avgCostDelta, tier)

    const runsPerMonth = runsInRange.length * 30 / Math.max(periodDays, 1)
    const monthlyCurrentSpend = runsInRange.reduce((total, run) => {
      const rec = run.recommendations?.[0]
      if (!rec || rec.current_monthly_usd <= 0) return total
      const durationHours = run.duration_seconds / 3600
      const scaleFactor = (durationHours * runsPerMonth) / 720
      return total + rec.current_monthly_usd * scaleFactor
    }, 0)
    const monthlySavings = runsInRange.reduce((total, run) => {
      const rec = run.recommendations?.[0]
      if (!rec || rec.cost_delta_percent >= -0.5) return total
      const durationHours = run.duration_seconds / 3600
      const scaleFactor = (durationHours * runsPerMonth) / 720
      const adjustedSavings = Math.max(0, rec.current_monthly_usd - rec.estimated_monthly_usd) * scaleFactor
      return total + adjustedSavings
    }, 0)

    return {
      jobId,
      repository,
      ciPlatform,
      provider: mode(providers) ?? '',
      detectedMachine: mode(detected) ?? '',
      suggestedMachine: mode(suggested) ?? '',
      tier,
      availableTiers,
      runCount: runsInRange.length,
      medianCpu: median(cpu),
      medianMem: median(mem),
      medianDuration: median(dur),
      avgCostDelta,
      avgSpotDelta,
      spotRisk,
      benefitLabel: benefit.label,
      benefitTone: benefit.tone,
      monthlyCurrentSpend,
      monthlySavings,
      latestDate: latestOverallDate,
    }
  })
}

export function pgBtnStyle(disabled: boolean, active = false): React.CSSProperties {
  return {
    background: active ? '#C23B22' : 'transparent',
    color: active ? '#FBF0DC' : disabled ? '#D4B896' : '#6B4226',
    border: '1px solid',
    borderColor: active ? '#C23B22' : '#D4B896',
    padding: '4px 10px',
    fontFamily: "'Bebas Neue', Impact, sans-serif",
    fontSize: 14,
    cursor: disabled ? 'default' : 'pointer',
    opacity: disabled ? 0.4 : 1,
    minWidth: 32,
    transition: 'all .1s',
    boxShadow: active ? '2px 2px 0 rgba(92,58,30,.15)' : 'none',
  }
}

export function pageNumbers(current: number, total: number): (number | null)[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1)
  const pages: (number | null)[] = [1]
  if (current > 3) pages.push(null)
  for (let p = Math.max(2, current - 1); p <= Math.min(total - 1, current + 1); p++) pages.push(p)
  if (current < total - 2) pages.push(null)
  pages.push(total)
  return pages
}
