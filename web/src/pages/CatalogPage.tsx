import { useEffect, useState, useMemo } from 'react'
import { fetchCatalog } from '../api'
import type { MachineType } from '../types'
import { useDebounce } from '../hooks/useDebounce'
import { formatFromUSD, useCurrencyPreference } from '../currency'

export default function CatalogPage() {
  const { currency } = useCurrencyPreference()
  const [machines, setMachines] = useState<MachineType[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const debouncedSearch = useDebounce(search)
  const [provider, setProvider] = useState('')
  const [arch, setArch] = useState('')
  const [minVcpus, setMinVcpus] = useState('')
  const [minMemoryGiB, setMinMemoryGiB] = useState('')
  const [minNetworkGbps, setMinNetworkGbps] = useState('')
  const [storageType, setStorageType] = useState('')
  const [sortKey, setSortKey] = useState<keyof MachineType>('on_demand_price_per_hour')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('asc')
  const [page,     setPage]     = useState(1)
  const [pageSize, setPageSize] = useState(10)

  useEffect(() => {
    fetchCatalog(provider || undefined)
      .then(setMachines)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [provider])

  const filtered = useMemo(() => {
    let list = machines
    if (debouncedSearch) {
      const q = debouncedSearch.toLowerCase()
      list = list.filter((m) => m.id.toLowerCase().includes(q) || m.family.toLowerCase().includes(q) || m.series.toLowerCase().includes(q))
    }
    if (arch) list = list.filter((m) => m.architecture === arch)
    if (minVcpus) list = list.filter((m) => m.vcpus >= Number(minVcpus))
    if (minMemoryGiB) list = list.filter((m) => m.memory_gib >= Number(minMemoryGiB))
    if (minNetworkGbps) list = list.filter((m) => m.network_gbps >= Number(minNetworkGbps))
    if (storageType) list = list.filter((m) => m.storage_type === storageType)
    return [...list].sort((a, b) => {
      const av = a[sortKey] as number | string
      const bv = b[sortKey] as number | string
      if (av < bv) return sortDir === 'asc' ? -1 : 1
      if (av > bv) return sortDir === 'asc' ? 1 : -1
      return 0
    })
  }, [machines, debouncedSearch, arch, minVcpus, minMemoryGiB, minNetworkGbps, storageType, sortKey, sortDir])

  const vcpuOptions = useMemo(
    () => [...new Set(machines.map((m) => m.vcpus).filter((v) => v > 0))].sort((a, b) => a - b),
    [machines],
  )

  const networkOptions = useMemo(
    () => [...new Set(machines.map((m) => m.network_gbps).filter((v) => v > 0))].sort((a, b) => a - b),
    [machines],
  )

  const memoryOptions = useMemo(
    () => [...new Set(machines.map((m) => m.memory_gib).filter((v) => v > 0))].sort((a, b) => a - b),
    [machines],
  )

  const storageTypeOptions = useMemo(
    () => [...new Set(machines.map((m) => m.storage_type).filter(Boolean))].sort(),
    [machines],
  )

  useEffect(() => { setPage(1) }, [filtered, pageSize])

  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize))
  const paginated  = filtered.slice((page - 1) * pageSize, page * pageSize)

  function toggleSort(key: keyof MachineType) {
    if (sortKey === key) setSortDir((d) => d === 'asc' ? 'desc' : 'asc')
    else { setSortKey(key); setSortDir('asc') }
  }

  function SortIndicator({ col }: { col: keyof MachineType }) {
    if (sortKey !== col) return null
    return <span style={{ marginLeft: 4 }}>{sortDir === 'asc' ? '↑' : '↓'}</span>
  }

  if (loading) return <div className="empty">Loading catalog…</div>
  if (error) return <div className="empty">Error: {error}</div>

  const hasActiveFilters = Boolean(search || provider || arch || minVcpus || minMemoryGiB || minNetworkGbps || storageType)

  const clearFilters = () => {
    setSearch('')
    setProvider('')
    setArch('')
    setMinVcpus('')
    setMinMemoryGiB('')
    setMinNetworkGbps('')
    setStorageType('')
    setPage(1)
  }

  return (
    <div className="fadein">
      <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] mb-7 tracking-tight">Machine Catalog</h1>
      <div className="flex flex-wrap gap-2 mb-5 items-center">
        <input
          className="rr-input !w-auto min-w-[200px] flex-1 sm:flex-none sm:w-[260px]"
          placeholder="Search by ID, family, series…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <select className="rr-select flex-1 sm:flex-none" value={provider} onChange={(e) => { setProvider(e.target.value); setLoading(true) }}>
          <option value="">All providers</option>
          <option value="aws">AWS</option>
          <option value="gcp">GCP</option>
          <option value="github">GitHub</option>
        </select>
        <select className="rr-select flex-1 sm:flex-none" value={arch} onChange={(e) => setArch(e.target.value)}>
          <option value="">All architectures</option>
          <option value="x86_64">x86_64</option>
          <option value="arm64">arm64</option>
        </select>
        <select className="rr-select flex-1 sm:flex-none" value={minVcpus} onChange={(e) => setMinVcpus(e.target.value)}>
          <option value="">Any vCPU count</option>
          {vcpuOptions.map((v) => (
            <option key={v} value={v}>{`>= ${v} vCPU${v === 1 ? '' : 's'}`}</option>
          ))}
        </select>
        <select className="rr-select flex-1 sm:flex-none" value={minMemoryGiB} onChange={(e) => setMinMemoryGiB(e.target.value)}>
          <option value="">Any memory</option>
          {memoryOptions.map((v) => (
            <option key={v} value={v}>{`>= ${v} GiB`}</option>
          ))}
        </select>
        <select
          className="rr-select flex-1 sm:flex-none"
          value={minNetworkGbps}
          onChange={(e) => setMinNetworkGbps(e.target.value)}
        >
          <option value="">Any network</option>
          {networkOptions.map((v) => (
            <option key={v} value={v}>{`>= ${v} Gbps`}</option>
          ))}
        </select>
        <select className="rr-select flex-1 sm:flex-none" value={storageType} onChange={(e) => setStorageType(e.target.value)}>
          <option value="">Any storage</option>
          {storageTypeOptions.map((s) => (
            <option key={s} value={s}>{s}</option>
          ))}
        </select>
        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="border border-[var(--border)] text-[var(--text-light)] font-deco text-[13px] tracking-[1px] px-3 py-2 bg-transparent hover:border-[var(--border-dark)] hover:text-[var(--text-mid)] transition-colors cursor-pointer"
          >
            Clear
          </button>
        )}
        <span className="font-deco text-[14px] tracking-[2px] text-[var(--text-light)] self-center ml-auto">
          {filtered.length} machines
        </span>
      </div>
      <div className="sm:hidden grid gap-3 mb-4">
        {paginated.map((m) => (
          <div key={`${m.provider}-${m.id}`} className="bg-paper border border-[var(--border)] rounded-lg px-4 py-3 shadow-rr">
            <div className="flex items-start justify-between gap-3 mb-2">
              <div>
                <div className="font-mono text-[13px] text-[var(--text)] break-all">{m.id}</div>
                <div className="text-xs text-[var(--text-light)] mt-1">{m.provider.toUpperCase()} · {m.architecture}</div>
              </div>
              <span className={`badge badge-${m.provider}`}>{m.provider.toUpperCase()}</span>
            </div>
            <div className="grid grid-cols-2 gap-2 text-xs text-[var(--text-mid)]">
              <div><span className="text-[var(--text-light)]">vCPUs</span><div>{m.vcpus}</div></div>
              <div><span className="text-[var(--text-light)]">Memory</span><div>{m.memory_gib} GiB</div></div>
              <div><span className="text-[var(--text-light)]">/hr</span><div>{formatFromUSD(m.on_demand_price_per_hour, currency, { minimumFractionDigits: 4, maximumFractionDigits: 4 })}</div></div>
              <div><span className="text-[var(--text-light)]">/mo</span><div>{formatFromUSD(m.on_demand_price_per_hour * 720, currency)}</div></div>
              <div className="col-span-2"><span className="text-[var(--text-light)]">Family</span><div>{m.family}</div></div>
              <div className="col-span-2"><span className="text-[var(--text-light)]">Tags</span><div className="break-all">{m.tags?.join(', ')}</div></div>
            </div>
          </div>
        ))}
      </div>

      <div className="rr-card !p-0 hidden sm:block">
        <div className="table-wrap">
          <table className="rr-table">
            <thead>
              <tr>
                <th className="cursor-pointer" onClick={() => toggleSort('id')}>ID <SortIndicator col="id" /></th>
                <th>Provider</th>
                <th className="hidden md:table-cell">Family</th>
                <th className="cursor-pointer" onClick={() => toggleSort('vcpus')}>vCPUs <SortIndicator col="vcpus" /></th>
                <th className="cursor-pointer" onClick={() => toggleSort('memory_gib')}>Memory <SortIndicator col="memory_gib" /></th>
                <th className="hidden lg:table-cell">Network</th>
                <th className="hidden sm:table-cell">Arch</th>
                <th className="cursor-pointer" onClick={() => toggleSort('on_demand_price_per_hour')}>Price/hr <SortIndicator col="on_demand_price_per_hour" /></th>
                <th className="hidden sm:table-cell">Price/month</th>
                <th className="hidden xl:table-cell">Tags</th>
              </tr>
            </thead>
            <tbody>
              {paginated.map((m) => (
                <tr key={`${m.provider}-${m.id}`}>
                  <td><code className="text-[13px] font-mono">{m.id}</code></td>
                  <td><span className={`badge badge-${m.provider}`}>{m.provider.toUpperCase()}</span></td>
                  <td className="hidden md:table-cell">{m.family}</td>
                  <td>{m.vcpus}</td>
                  <td>{m.memory_gib} GiB</td>
                  <td className="hidden lg:table-cell">{m.network_gbps > 0 ? `${m.network_gbps} Gbps` : null}</td>
                  <td className="hidden sm:table-cell">{m.architecture}</td>
                  <td>{formatFromUSD(m.on_demand_price_per_hour, currency, { minimumFractionDigits: 4, maximumFractionDigits: 4 })}</td>
                  <td className="hidden sm:table-cell">{formatFromUSD(m.on_demand_price_per_hour * 720, currency)}</td>
                  <td className="hidden xl:table-cell text-[11px] text-[var(--text-light)]">{m.tags?.join(', ')}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {totalPages > 1 && (
        <div className="flex flex-wrap items-center justify-between gap-3 mt-3">
          <div className="flex items-center gap-2">
            <span className="font-deco text-[12px] tracking-[1.5px] text-[var(--text-light)]">ROWS</span>
            {CAT_PAGE_SIZES.map(s => (
              <button key={s} onClick={() => setPageSize(s)} style={catPgBtn(false, pageSize === s)}>{s}</button>
            ))}
          </div>
          <div className="flex items-center gap-1.5 flex-wrap">
            <span className="font-deco text-[12px] tracking-[1px] text-[var(--text-light)] mr-2">
              {(page - 1) * pageSize + 1}–{Math.min(page * pageSize, filtered.length)} of {filtered.length}
            </span>
            <button onClick={() => setPage(1)} disabled={page === 1} style={catPgBtn(page === 1)}>«</button>
            <button onClick={() => setPage(p => p - 1)} disabled={page === 1} style={catPgBtn(page === 1)}>‹</button>
            {catPageNums(page, totalPages).map((p, i) =>
              p === null
                ? <span key={`e-${i}`} className="text-[var(--text-light)] px-1 font-deco">…</span>
                : <button key={p} onClick={() => setPage(p)} style={catPgBtn(false, p === page)}>{p}</button>
            )}
            <button onClick={() => setPage(p => p + 1)} disabled={page === totalPages} style={catPgBtn(page === totalPages)}>›</button>
            <button onClick={() => setPage(totalPages)} disabled={page === totalPages} style={catPgBtn(page === totalPages)}>»</button>
          </div>
        </div>
      )}
    </div>
  )
}

const CAT_PAGE_SIZES = [10, 20, 50, 100]

function catPgBtn(disabled: boolean, active = false): React.CSSProperties {
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

function catPageNums(current: number, total: number): (number | null)[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1)
  const pages: (number | null)[] = [1]
  if (current > 3) pages.push(null)
  for (let p = Math.max(2, current - 1); p <= Math.min(total - 1, current + 1); p++) pages.push(p)
  if (current < total - 2) pages.push(null)
  pages.push(total)
  return pages
}
