import { useEffect, useMemo, useState } from 'react'
import { fetchJobs } from '../api'
import type { Job, SavingsHistoryPoint } from '../types'
import { DateRangePicker, inDateRange, EMPTY_RANGE } from '../components/DateRangePicker'
import type { DateRange } from '../components/DateRangePicker'
import { useDebounce } from '../hooks/useDebounce'
import { useCurrencyPreference } from '../currency'
import SavingsBanner from './JobsPage/SavingsBanner'
import SavingsChart from './JobsPage/SavingsChart'
import JobsTable from './JobsPage/JobsTable'
import Pagination from './JobsPage/Pagination'
import { groupJobs, round2 } from './JobsPage/utils'
import type { SortKey } from './JobsPage/types'

export default function JobsPage() {
  const { currency } = useCurrencyPreference()
  const [jobs, setJobs] = useState<Job[]>([])
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounce(search)
  const [repository, setRepository] = useState('')
  const [ciPlatform, setCiPlatform] = useState('')
  const [provider, setProvider] = useState('')
  const [tier, setTier] = useState('')
  const [dateRange, setDateRange] = useState<DateRange>(EMPTY_RANGE)

  const [sortKey, setSortKey] = useState<SortKey>('latestDate')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('desc')
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(10)

  async function loadJobs(showInitialLoader = false) {
    if (showInitialLoader) setLoading(true)
    else setRefreshing(true)
    setError(null)
    try {
      const data = await fetchJobs()
      setJobs(data)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      if (showInitialLoader) setLoading(false)
      else setRefreshing(false)
    }
  }

  useEffect(() => {
    void loadJobs(true)
  }, [])

  // Reset to page 1 on filter/page-size change
  useEffect(() => {
    setPage(1)
  }, [debouncedSearch, repository, ciPlatform, provider, tier, dateRange, pageSize])

  function toggleSort(key: SortKey) {
    if (sortKey === key) setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    else {
      setSortKey(key)
      setSortDir('desc')
    }
    setPage(1)
  }

  const groups = useMemo(() => groupJobs(jobs, dateRange), [jobs, dateRange])

  const filtered = useMemo(() => {
    let list = groups
    if (debouncedSearch) list = list.filter((g) => g.jobId.toLowerCase().includes(debouncedSearch.toLowerCase()))
    if (repository) list = list.filter((g) => g.repository === repository)
    if (ciPlatform) list = list.filter((g) => g.ciPlatform === ciPlatform)
    if (provider) list = list.filter((g) => g.provider === provider)
    if (tier) list = list.filter((g) => g.availableTiers.includes(tier))
    return [...list].sort((a, b) => {
      let av = 0,
        bv = 0
      switch (sortKey) {
        case 'latestDate':
          av = new Date(a.latestDate).getTime()
          bv = new Date(b.latestDate).getTime()
          break
        case 'medianCpu':
          av = a.medianCpu
          bv = b.medianCpu
          break
        case 'medianMem':
          av = a.medianMem
          bv = b.medianMem
          break
        case 'medianDuration':
          av = a.medianDuration
          bv = b.medianDuration
          break
        case 'avgCostDelta':
          av = a.avgCostDelta
          bv = b.avgCostDelta
          break
        case 'runCount':
          av = a.runCount
          bv = b.runCount
          break
      }
      return sortDir === 'asc' ? av - bv : bv - av
    })
  }, [groups, debouncedSearch, repository, ciPlatform, provider, tier, sortKey, sortDir])

  const filteredRuns = useMemo(() => {
    return jobs.filter((job) => {
      if (!inDateRange(job.start_time, dateRange)) return false
      if (debouncedSearch && !job.job_id.toLowerCase().includes(debouncedSearch.toLowerCase())) return false
      if (repository && (job.repository ?? '') !== repository) return false
      if (ciPlatform && (job.summary?.ci_platform ?? '') !== ciPlatform) return false
      if (provider && (job.summary?.detected_machine?.provider ?? '') !== provider) return false
      if (tier && !(job.recommendations ?? []).some((rec) => rec.tier === tier)) return false
      return true
    })
  }, [jobs, dateRange, debouncedSearch, repository, ciPlatform, provider, tier])

  const savingsSnapshot = useMemo(() => {
    const totalJobs = filtered.length
    const savingsJobs = filtered.filter((g) => g.monthlySavings > 0)
    const jobsWithSavings = savingsJobs.length
    const estimatedCurrentMonthlySpend = round2(filtered.reduce((sum, g) => sum + g.monthlyCurrentSpend, 0))
    const estimatedCurrentAnnualSpend = round2(estimatedCurrentMonthlySpend * 12)
    const estimatedMonthlySavings = round2(filtered.reduce((sum, g) => sum + g.monthlySavings, 0))
    const projectedAnnualSavings = round2(estimatedMonthlySavings * 12)
    const avgWastePercent =
      jobsWithSavings > 0
        ? savingsJobs.reduce((sum, g) => sum + Math.abs(Math.min(g.avgCostDelta, 0)), 0) / jobsWithSavings
        : 0
    return {
      totalJobs,
      jobsWithSavings,
      estimatedCurrentMonthlySpend,
      estimatedCurrentAnnualSpend,
      estimatedMonthlySavings,
      projectedAnnualSavings,
      avgWastePercent,
    }
  }, [filtered])

  const savingsHistory = useMemo<SavingsHistoryPoint[]>(() => {
    const byDay = new Map<string, { monthly: number; jobs: Set<string> }>()
    for (const run of filteredRuns) {
      const day = new Date(run.start_time).toISOString().slice(0, 10)
      const rec = run.recommendations?.[0]
      const monthly =
        rec && rec.cost_delta_percent < -0.5
          ? Math.max(0, rec.current_monthly_usd - rec.estimated_monthly_usd)
          : 0
      const prev = byDay.get(day) ?? { monthly: 0, jobs: new Set<string>() }
      prev.monthly += monthly
      prev.jobs.add(run.job_id)
      byDay.set(day, prev)
    }
    return [...byDay.entries()]
      .sort((a, b) => a[0].localeCompare(b[0]))
      .map(([date, data]) => ({ date, job_count: data.jobs.size, monthly_savings: round2(data.monthly) }))
  }, [filteredRuns])

  const allRepositories = useMemo(
    () => [...new Set(groups.map((g) => g.repository).filter(Boolean))].sort(),
    [groups]
  )

  const paginated = filtered.slice((page - 1) * pageSize, page * pageSize)

  const clearFilters = () => {
    setSearch('')
    setRepository('')
    setCiPlatform('')
    setProvider('')
    setTier('')
    setDateRange(EMPTY_RANGE)
  }

  if (loading) return <div className="empty">Loading jobs…</div>
  if (error) return <div className="empty">Error: {error}</div>

  return (
    <div className="fadein">
      <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] mb-7 tracking-tight">Jobs</h1>

      {filtered.length > 0 && <SavingsBanner snapshot={savingsSnapshot} currency={currency} />}

      <SavingsChart history={savingsHistory} currency={currency} />

      {/* Filter bar */}
      <div className="flex flex-wrap gap-2 mb-5 items-start sm:items-center">
        <input
          className="rr-input !w-full sm:!w-auto min-w-0 sm:min-w-[180px] flex-1 sm:flex-none sm:w-[200px]"
          placeholder="Search job name…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        {allRepositories.length > 0 && (
          <select
            className="rr-select flex-1 sm:flex-none"
            value={repository}
            onChange={(e) => setRepository(e.target.value)}
          >
            <option value="">All repositories</option>
            {allRepositories.map((r) => (
              <option key={r} value={r}>
                {r}
              </option>
            ))}
          </select>
        )}
        <select
          className="rr-select flex-1 sm:flex-none"
          value={ciPlatform}
          onChange={(e) => setCiPlatform(e.target.value)}
        >
          <option value="">All CI platforms</option>
          <option value="github">GitHub Actions</option>
          <option value="gitlab">GitLab CI</option>
          <option value="circleci">CircleCI</option>
          <option value="bitbucket">Bitbucket Pipelines</option>
          <option value="azure">Azure Pipelines</option>
          <option value="jenkins">Jenkins</option>
          <option value="local">Local</option>
        </select>
        <select
          className="rr-select flex-1 sm:flex-none"
          value={provider}
          onChange={(e) => setProvider(e.target.value)}
        >
          <option value="">All providers</option>
          <option value="aws">AWS</option>
          <option value="gcp">GCP</option>
          <option value="github">GitHub</option>
        </select>
        <select className="rr-select flex-1 sm:flex-none" value={tier} onChange={(e) => setTier(e.target.value)}>
          <option value="">All tiers</option>
          <option value="right-sized">Right-sized</option>
          <option value="cheaper-option">Cheaper option</option>
          <option value="more-headroom">More headroom</option>
        </select>
        {(search || repository || ciPlatform || provider || tier) && (
          <button
            onClick={clearFilters}
            className="border border-[var(--border)] text-[var(--text-light)] font-deco text-[13px] tracking-[1px] px-3 py-2 bg-transparent hover:border-[var(--border-dark)] hover:text-[var(--text-mid)] transition-colors cursor-pointer"
          >
            Clear
          </button>
        )}
        <button
          onClick={() => void loadJobs(false)}
          disabled={refreshing}
          className="border border-[var(--border)] text-[var(--text-light)] font-deco text-[13px] tracking-[1px] px-3 py-2 bg-transparent hover:border-[var(--border-dark)] hover:text-[var(--text-mid)] transition-colors cursor-pointer disabled:opacity-60 disabled:cursor-not-allowed"
        >
          {refreshing ? 'Refreshing…' : 'Refresh'}
        </button>
        <span className="w-full sm:w-auto sm:ml-auto font-deco text-[13px] tracking-[1px] text-[var(--text-light)] self-center whitespace-nowrap text-right sm:text-left">
          {filtered.length} {filtered.length === 1 ? 'job' : 'jobs'}
        </span>
      </div>

      <div className="mb-5">
        <DateRangePicker value={dateRange} onChange={(r) => { setDateRange(r); setPage(1) }} />
      </div>

      {jobs.length === 0 ? (
        <div className="empty">
          No jobs yet. Run <code>runright monitor</code> with <code>--export http</code> to send metrics here.
        </div>
      ) : filtered.length === 0 ? (
        <div className="empty text-base">No jobs match your filters.</div>
      ) : (
        <>
          <JobsTable
            jobs={paginated}
            sortKey={sortKey}
            sortDir={sortDir}
            currency={currency}
            onSort={toggleSort}
          />
          <Pagination
            page={page}
            pageSize={pageSize}
            total={filtered.length}
            onPageChange={setPage}
            onPageSizeChange={setPageSize}
          />
        </>
      )}
    </div>
  )
}
