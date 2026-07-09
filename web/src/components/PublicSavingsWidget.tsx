import { useState, useEffect } from 'react'

interface PublicStats {
  total_jobs: number
  total_orgs: number
  total_savings_usd: number
  total_carbon_saved_kg: number
  last_updated: string
  by_provider?: { provider: string; job_count: number; savings_usd: number }[]
}

/**
 * Public Savings Counter Widget
 * Can be embedded on landing pages, README badges, or marketing materials.
 * Fetches unauthenticated stats from /api/v1/public/stats
 */
export default function PublicSavingsWidget({
  variant = 'full',
  className = '',
}: {
  variant?: 'full' | 'compact' | 'badge'
  className?: string
}) {
  const [stats, setStats] = useState<PublicStats | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    fetch('/api/v1/public/stats')
      .then((res) => res.ok ? res.json() : null)
      .then(setStats)
      .catch(() => setStats(null))
      .finally(() => setLoading(false))
  }, [])

  // Animate counter effect
  const [displaySavings, setDisplaySavings] = useState(0)
  useEffect(() => {
    if (!stats?.total_savings_usd) return
    const target = stats.total_savings_usd
    const duration = 2000
    const steps = 60
    const increment = target / steps
    let current = 0
    const timer = setInterval(() => {
      current += increment
      if (current >= target) {
        setDisplaySavings(target)
        clearInterval(timer)
      } else {
        setDisplaySavings(current)
      }
    }, duration / steps)
    return () => clearInterval(timer)
  }, [stats?.total_savings_usd])

  if (loading) {
    return (
      <div className={`animate-pulse ${className}`}>
        <div className="h-16 bg-gray-200 rounded-lg" />
      </div>
    )
  }

  if (!stats) return null

  // Badge variant - minimal inline display
  if (variant === 'badge') {
    return (
      <span className={`inline-flex items-center gap-1.5 px-3 py-1 bg-green-100 text-green-800 rounded-full text-sm font-medium ${className}`}>
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <polyline points="23 18 13.5 8.5 8.5 13.5 1 6" />
        </svg>
        ${formatNumber(stats.total_savings_usd)} saved
      </span>
    )
  }

  // Compact variant - small card
  if (variant === 'compact') {
    return (
      <div className={`bg-white rounded-lg shadow-sm border p-4 ${className}`}>
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xs text-gray-500 uppercase tracking-wide">Community Savings</p>
            <p className="text-2xl font-bold text-green-600">${formatNumber(displaySavings)}</p>
          </div>
          <div className="text-right">
            <p className="text-xs text-gray-500">{stats.total_jobs.toLocaleString()} jobs</p>
            <p className="text-xs text-gray-500">{stats.total_orgs.toLocaleString()} orgs</p>
          </div>
        </div>
      </div>
    )
  }

  // Full variant - detailed display
  return (
    <div className={`bg-gradient-to-br from-green-50 to-emerald-50 rounded-xl shadow-lg border border-green-100 p-6 ${className}`}>
      <div className="text-center mb-6">
        <p className="text-sm text-green-700 font-medium uppercase tracking-wide mb-2">
          Total Community Savings
        </p>
        <p className="text-5xl font-black text-green-700">
          ${formatNumber(displaySavings)}
        </p>
        <p className="text-sm text-green-600 mt-2">
          per month across all RunRight users
        </p>
      </div>

      <div className="grid grid-cols-3 gap-4 mb-6">
        <StatBox
          label="Jobs Optimized"
          value={stats.total_jobs.toLocaleString()}
          icon={<JobIcon />}
        />
        <StatBox
          label="Organizations"
          value={stats.total_orgs.toLocaleString()}
          icon={<OrgIcon />}
        />
        <StatBox
          label="CO₂ Saveable"
          value={`${stats.total_carbon_saved_kg.toFixed(0)} kg`}
          icon={<LeafIcon />}
        />
      </div>

      {stats.by_provider && stats.by_provider.length > 0 && (
        <div className="border-t border-green-200 pt-4">
          <p className="text-xs text-green-600 mb-2 font-medium">By Provider</p>
          <div className="flex flex-wrap gap-2">
            {stats.by_provider.slice(0, 4).map((p) => (
              <span
                key={p.provider}
                className="inline-flex items-center gap-1 px-2 py-1 bg-white rounded text-xs text-gray-700"
              >
                <span className="font-medium">{p.provider || 'Other'}</span>
                <span className="text-gray-400">•</span>
                <span className="text-green-600">${formatNumber(p.savings_usd)}</span>
              </span>
            ))}
          </div>
        </div>
      )}

      <div className="mt-4 text-center">
        <a
          href="https://github.com/runright/runright"
          className="inline-flex items-center gap-1.5 text-sm text-green-700 hover:text-green-900 font-medium"
        >
          Join the movement
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14 5l7 7m0 0l-7 7m7-7H3" />
          </svg>
        </a>
      </div>
    </div>
  )
}

function formatNumber(n: number): string {
  if (n >= 1000000) return `${(n / 1000000).toFixed(1)}M`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`
  return n.toFixed(0)
}

function StatBox({
  label,
  value,
  icon,
}: {
  label: string
  value: string
  icon: React.ReactNode
}) {
  return (
    <div className="text-center">
      <div className="w-6 h-6 mx-auto mb-1 text-green-600">{icon}</div>
      <p className="text-lg font-bold text-gray-900">{value}</p>
      <p className="text-xs text-gray-500">{label}</p>
    </div>
  )
}

function JobIcon() {
  return (
    <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <rect x="3" y="3" width="18" height="18" rx="2" strokeWidth={2} />
      <path d="M9 9h6M9 15h6" strokeWidth={2} />
    </svg>
  )
}

function OrgIcon() {
  return (
    <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" strokeWidth={2} />
      <circle cx="9" cy="7" r="4" strokeWidth={2} />
      <path d="M23 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75" strokeWidth={2} />
    </svg>
  )
}

function LeafIcon() {
  return (
    <svg fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path d="M11 20A7 7 0 0 1 9.8 6.1C15.5 5 17 4.48 19 2c1 2 2 4.18 2 8 0 5.5-4.78 10-10 10Z" strokeWidth={2} />
      <path d="M2 21c0-3 1.85-5.36 5.08-6C9.5 14.52 12 13 13 12" strokeWidth={2} />
    </svg>
  )
}
