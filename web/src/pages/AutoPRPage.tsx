import { useEffect, useState, useMemo, useRef, useCallback } from 'react'
import {
  fetchLabelMappings, fetchAutoPRSettings, fetchPRRecommendations, fetchPRHistory,
  fetchGPUTiers, upsertLabelMapping, deleteLabelMapping, upsertAutoPRSettings,
  approvePRRecommendation, dismissPRRecommendation, fetchCatalog, fetchRepos,
  triggerAutoPRScan,
  type LabelMapping, type AutoPRSettings, type PRRecommendation, type GPUTier, type PRHistory
} from '../api'
import { useAutoPRWebSocket } from '../hooks/useWebSocket'
import { usePagination } from '../hooks/usePagination'
import { ListControls } from '../components/ListControls'
import type { MachineType, RepoSummary } from '../types'

type TabId = 'recommendations' | 'mappings' | 'settings' | 'history'

export default function AutoPRPage() {
  const [activeTab, setActiveTab] = useState<TabId>('mappings')
  const [mappings, setMappings] = useState<LabelMapping[]>([])
  const [settings, setSettings] = useState<AutoPRSettings | null>(null)
  const [recommendations, setRecommendations] = useState<PRRecommendation[]>([])
  const [history, setHistory] = useState<PRHistory[]>([])
  const [gpuTiers, setGpuTiers] = useState<GPUTier[]>([])
  const [catalog, setCatalog] = useState<MachineType[]>([])
  const [repos, setRepos] = useState<RepoSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [gpuFilter, setGpuFilter] = useState(false)
  const [statusFilter, setStatusFilter] = useState('pending')
  const [error, setError] = useState('')
  const [note, setNote] = useState('')
  const [approveErrors, setApproveErrors] = useState<Record<string, string>>({})
  const [approving, setApproving] = useState<Record<string, boolean>>({})

  const loadData = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const [m, s, r, h, g, c, rp] = await Promise.all([
        fetchLabelMappings(gpuFilter),
        fetchAutoPRSettings(),
        fetchPRRecommendations(statusFilter, gpuFilter),
        fetchPRHistory(undefined, gpuFilter),
        fetchGPUTiers(),
        fetchCatalog(),
        fetchRepos(),
      ])
      setMappings(m)
      setSettings(s)
      setRecommendations(r)
      setHistory(h)
      setGpuTiers(g)
      setCatalog(c)
      setRepos(rp)
    } catch (err) {
      setError('Failed to load data')
      console.error(err)
    }
    setLoading(false)
  }, [gpuFilter, statusFilter])

  // Subscribe to WebSocket for realtime updates
  const { isConnected } = useAutoPRWebSocket(loadData)

  useEffect(() => {
    loadData()
  }, [loadData])

  useEffect(() => {
    if (note) {
      const t = setTimeout(() => setNote(''), 5000) // Increased to 5s so users can read PR URLs
      return () => clearTimeout(t)
    }
  }, [note])

  const handleApprove = async (id: string) => {
    // Clear any previous error for this card and mark as in-progress
    setApproveErrors((prev) => { const n = {...prev}; delete n[id]; return n })
    setApproving((prev) => ({ ...prev, [id]: true }))
    try {
      const result = await approvePRRecommendation(id)
      if (result.pr_url) {
        setNote(`PR created: ${result.pr_url}`)
      } else {
        // Shouldn't happen — means backend approved but no pr_url was returned
        setApproveErrors((prev) => ({ ...prev, [id]: 'PR approved but no URL returned — check server logs for GitHub API errors.' }))
      }
      loadData()
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error
        ?? 'Failed to approve — check that the server is reachable'
      setApproveErrors((prev) => ({ ...prev, [id]: msg }))
      loadData()
    } finally {
      setApproving((prev) => { const n = {...prev}; delete n[id]; return n })
    }
  }

  const handleDismiss = async (id: string) => {
    const reason = prompt('Dismiss reason (optional):')
    try {
      await dismissPRRecommendation(id, reason || '')
      setNote('Recommendation dismissed')
      loadData()
    } catch {
      setError('Failed to dismiss')
    }
  }

  const handleSaveSettings = async (s: AutoPRSettings) => {
    try {
      await upsertAutoPRSettings(s)
      setNote('Settings saved')
      loadData()
    } catch {
      setError('Failed to save settings')
    }
  }

  const tabs: { id: TabId; label: string }[] = [
    { id: 'recommendations', label: 'Recommendations' },
    { id: 'mappings', label: 'Label Mappings' },
    { id: 'settings', label: 'Settings' },
    { id: 'history', label: 'History' },
  ]

  return (
    <div className="fadein">
      <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] mb-2 tracking-tight">
        Auto PR Optimization
      </h1>
      <p className="text-[var(--text-light)] mb-6 text-sm">
        Configure label mappings and auto-generate PRs for CI cost savings
      </p>

      {error && (
        <div className="mb-4 p-3 bg-[rgba(194,59,34,.08)] border border-[var(--red)] text-[var(--red)] text-sm">
          {error}
        </div>
      )}
      {note && (
        <div className="mb-4 p-3 bg-[rgba(46,125,50,.08)] border border-[#2E7D32] text-[#2E7D32] text-sm">
          {note}
        </div>
      )}

      {/* Tabs */}
      <div className="flex gap-0 mb-6 border-b-2 border-[var(--border)]">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`
              px-4 py-2.5 font-deco text-[12px] tracking-[1.5px] uppercase transition-colors
              border-b-2 -mb-[2px] cursor-pointer
              ${activeTab === tab.id 
                ? 'border-[var(--red)] text-[var(--red)] bg-[rgba(194,59,34,.04)]' 
                : 'border-transparent text-[var(--text-mid)] hover:text-[var(--text)] hover:bg-[var(--cream-alt)]'}
            `}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-3 mb-5 items-center">
        <label className="flex items-center gap-2 cursor-pointer group">
          <input
            type="checkbox"
            checked={gpuFilter}
            onChange={(e) => setGpuFilter(e.target.checked)}
            className="w-4 h-4 accent-[var(--red)]"
          />
          <span className="font-deco text-[11px] tracking-[1px] uppercase text-[var(--text-mid)] group-hover:text-[var(--text)]">
            GPU Jobs Only
          </span>
        </label>
        {activeTab === 'recommendations' && (
          <select
            className="rr-select"
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value)}
          >
            <option value="pending">Pending</option>
            <option value="approved">Approved</option>
            <option value="dismissed">Dismissed</option>
          </select>
        )}
      </div>

      {loading ? (
        <div className="rr-card text-center py-12 text-[var(--text-light)]">Loading…</div>
      ) : (
        <>
          {activeTab === 'recommendations' && (
            <RecommendationsTab
              recommendations={recommendations}
              approveErrors={approveErrors}
              approving={approving}
              onApprove={handleApprove}
              onDismiss={handleDismiss}
            />
          )}
          {activeTab === 'mappings' && (
            <MappingsTab
              mappings={mappings}
              gpuTiers={gpuTiers}
              catalog={catalog}
              repos={repos}
              onRefresh={loadData}
            />
          )}
          {activeTab === 'settings' && settings && (
            <SettingsTab
              settings={settings}
              onSave={handleSaveSettings}
            />
          )}
          {activeTab === 'history' && (
            <HistoryTab history={history} />
          )}
        </>
      )}
    </div>
  )
}

function RecommendationsTab({ recommendations, approveErrors, approving, onApprove, onDismiss }: {
  recommendations: PRRecommendation[]
  approveErrors: Record<string, string>
  approving: Record<string, boolean>
  onApprove: (id: string) => void
  onDismiss: (id: string) => void
}) {
  const pagination = usePagination({
    items: recommendations,
    pageSize: 10,
    searchFn: (item, query) => 
      item.repository.toLowerCase().includes(query) ||
      item.job_id.toLowerCase().includes(query) ||
      item.current_label.toLowerCase().includes(query) ||
      item.recommended_label.toLowerCase().includes(query),
  })

  if (recommendations.length === 0) {
    return (
      <div className="rr-card text-center py-12">
        <h3 className="font-serif text-lg text-[var(--text)] mb-2">No recommendations</h3>
        <p className="text-[var(--text-light)] text-sm">
          When jobs are detected as over-provisioned, recommendations will appear here.
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <ListControls 
        pagination={pagination} 
        searchPlaceholder="Search recommendations..." 
      />
      
      <div className="grid gap-4">
        {pagination.paginatedItems.map((rec) => (
          <div key={rec.id} className={`rr-card ${rec.is_gpu_job ? '!border-l-4 !border-l-[var(--gold)]' : ''}`}>
            <div className="flex justify-between items-start mb-4">
              <div>
                <span className="text-[var(--text-light)] text-xs font-mono">{rec.repository}</span>
                <h3 className="font-serif text-lg text-[var(--text)] mt-1">{rec.job_id}</h3>
              </div>
              {rec.is_gpu_job && <span className="badge badge-aws">GPU</span>}
            </div>

            <div className="grid grid-cols-[1fr_auto_1fr] gap-4 items-center mb-4">
              <div>
                <div className="font-deco text-[10px] tracking-[1px] text-[var(--text-light)] uppercase mb-1">Current</div>
                <code className="text-sm">{rec.current_label}</code>
                <div className="text-[var(--text-light)] text-xs mt-1">
                  {rec.current_vcpus} vCPU · {rec.current_memory_gib} GiB
                </div>
              </div>
              <div className="text-[var(--gold)] text-xl">→</div>
              <div>
                <div className="font-deco text-[10px] tracking-[1px] text-[var(--text-light)] uppercase mb-1">Recommended</div>
                <code className="text-sm text-[#2E7D32]">{rec.recommended_label}</code>
                <div className="text-[var(--text-light)] text-xs mt-1">
                  {rec.recommended_vcpus} vCPU · {rec.recommended_memory_gib} GiB
                </div>
              </div>
            </div>

            <div className="grid grid-cols-3 gap-4 p-3 bg-[var(--cream-alt)] mb-4">
              <div>
                <div className="font-deco text-[10px] tracking-[1px] text-[var(--text-light)] uppercase">p95 CPU</div>
                <div className="font-mono text-sm text-[var(--text)]">{rec.p95_cpu_percent.toFixed(1)}%</div>
              </div>
              <div>
                <div className="font-deco text-[10px] tracking-[1px] text-[var(--text-light)] uppercase">p95 Memory</div>
                <div className="font-mono text-sm text-[var(--text)]">{rec.p95_mem_percent.toFixed(1)}%</div>
              </div>
              {rec.p95_gpu_util_percent !== undefined && rec.p95_gpu_util_percent > 0 && (
                <div>
                  <div className="font-deco text-[10px] tracking-[1px] text-[var(--text-light)] uppercase">p95 GPU</div>
                  <div className="font-mono text-sm text-[var(--text)]">{rec.p95_gpu_util_percent.toFixed(1)}%</div>
                </div>
              )}
            </div>

            <div className="flex justify-between items-center">
              <div>
                <span className="text-2xl font-bold text-[#2E7D32]">-{rec.savings_percent.toFixed(0)}%</span>
                <span className="text-[var(--text-light)] text-sm ml-2">${rec.monthly_savings_usd.toFixed(2)}/mo</span>
              </div>
              <div className="text-[var(--text-light)] text-xs">
                {rec.run_count} runs · {rec.consecutive_underutilized} consecutive
              </div>
            </div>

            {rec.status === 'pending' && (
              <div className="flex gap-3 mt-4 pt-4 border-t border-[var(--border)]">
                <button className="btn-rr" disabled={approving[rec.id]} onClick={() => onApprove(rec.id)}>
                  {approving[rec.id] ? 'Creating PR…' : 'Approve & Create PR'}
                </button>
                <button 
                  className="px-4 py-2 border border-[var(--border)] text-[var(--text-mid)] font-deco text-[13px] tracking-[1px] hover:border-[var(--border-dark)] hover:text-[var(--text)] transition-colors cursor-pointer bg-transparent"
                  onClick={() => onDismiss(rec.id)}
                >
                  Dismiss
                </button>
              </div>
            )}

            {rec.status === 'approved' && (
              <div className="flex items-center gap-3 mt-4 pt-4 border-t border-[var(--border)]">
                <span className="badge badge-github">✓ Approved</span>
                {rec.pr_url ? (
                  <a 
                    href={rec.pr_url} 
                    target="_blank" 
                    rel="noopener noreferrer"
                    className="text-[var(--red)] hover:underline text-sm font-mono flex items-center gap-1"
                  >
                    <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 16 16">
                      <path d="M7.177 3.073L9.573.677A.25.25 0 0110 .854v4.792a.25.25 0 01-.427.177L7.177 3.427a.25.25 0 010-.354zM3.75 2.5a.75.75 0 100 1.5.75.75 0 000-1.5zm-2.25.75a2.25 2.25 0 113 2.122v5.256a2.251 2.251 0 11-1.5 0V5.372A2.25 2.25 0 011.5 3.25zM11 2.5h-1V4h1a1 1 0 011 1v5.628a2.251 2.251 0 101.5 0V5A2.5 2.5 0 0011 2.5zm1 10.25a.75.75 0 111.5 0 .75.75 0 01-1.5 0zM3.75 12a.75.75 0 100 1.5.75.75 0 000-1.5z"/>
                    </svg>
                    PR #{rec.pr_number || 'View'}
                  </a>
                ) : (
                  <div className="flex flex-col gap-1">
                    <div className="flex items-center gap-2">
                      <span className="text-[var(--text-light)] text-sm italic">PR creation pending</span>
                      <button
                        className="text-xs text-[var(--red)] hover:underline font-deco tracking-wide disabled:opacity-50 disabled:cursor-not-allowed"
                        disabled={approving[rec.id]}
                        onClick={() => onApprove(rec.id)}
                      >
                        {approving[rec.id] ? 'Creating PR…' : 'Retry'}
                      </button>
                    </div>
                    {approveErrors[rec.id] && (
                      <p className="text-xs text-[var(--red)] max-w-sm leading-snug">{approveErrors[rec.id]}</p>
                    )}
                  </div>
                )}
              </div>
            )}

            {rec.status === 'dismissed' && (
              <div className="flex items-center gap-3 mt-4 pt-4 border-t border-[var(--border)]">
                <span className="px-2 py-0.5 text-[10px] font-deco tracking-wider uppercase bg-[var(--cream-alt)] text-[var(--text-light)] border border-[var(--border)]">
                  Dismissed
                </span>
                {rec.dismissed_reason && (
                  <span className="text-[var(--text-light)] text-xs italic">"{rec.dismissed_reason}"</span>
                )}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

function MappingsTab({ mappings, gpuTiers, catalog, repos, onRefresh }: {
  mappings: LabelMapping[]
  gpuTiers: GPUTier[]
  catalog: MachineType[]
  repos: RepoSummary[]
  onRefresh: () => void
}) {
  const [showForm, setShowForm] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [form, setForm] = useState<Partial<LabelMapping>>({ repository: '*', is_gpu: false, gpu_count: 1 })
  const [saving, setSaving] = useState(false)
  const [errors, setErrors] = useState<Record<string, string>>({})
  const [instanceSearch, setInstanceSearch] = useState('')
  const [showDropdown, setShowDropdown] = useState(false)
  const [repoSearch, setRepoSearch] = useState('')
  const [showRepoDropdown, setShowRepoDropdown] = useState(false)
  const dropdownRef = useRef<HTMLDivElement>(null)
  const repoDropdownRef = useRef<HTMLDivElement>(null)

  // Pagination for mappings table
  const mappingsPagination = usePagination({
    items: mappings,
    pageSize: 10,
    searchFn: (item, query) =>
      item.label.toLowerCase().includes(query) ||
      item.repository.toLowerCase().includes(query) ||
      item.provider.toLowerCase().includes(query) ||
      item.instance_type.toLowerCase().includes(query),
  })

  // Close dropdowns on outside click
  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setShowDropdown(false)
      }
      if (repoDropdownRef.current && !repoDropdownRef.current.contains(e.target as Node)) {
        setShowRepoDropdown(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  // Filter repos by search
  const filteredRepos = useMemo(() => {
    if (!repoSearch.trim()) return repos
    const q = repoSearch.toLowerCase()
    return repos.filter(r => r.repository.toLowerCase().includes(q))
  }, [repos, repoSearch])

  // Filter catalog by provider and search term
  const filteredCatalog = useMemo(() => {
    if (!form.provider) return [] // Don't show anything if no provider selected
    let items = catalog.filter(m => m.provider === form.provider)
    if (instanceSearch.trim()) {
      const search = instanceSearch.toLowerCase()
      items = items.filter(m => 
        m.id.toLowerCase().includes(search) ||
        m.family.toLowerCase().includes(search) ||
        m.series.toLowerCase().includes(search)
      )
    }
    return items.slice(0, 50) // Limit to 50 results for performance
  }, [catalog, form.provider, instanceSearch])

  // Check if machine has GPU based on tags
  const machineHasGPU = (m: MachineType) => {
    return m.tags?.some(t => t.toLowerCase().includes('gpu') || t.toLowerCase().includes('nvidia') || t.toLowerCase().includes('tesla'))
  }

  const applyMachineType = (machine: MachineType) => {
    const isGPU = machineHasGPU(machine)
    setForm(prev => ({
      ...prev,
      provider: machine.provider,
      instance_type: machine.id,
      vcpus: machine.vcpus,
      memory_gib: machine.memory_gib,
      cost_per_hour: machine.on_demand_price_per_hour,
      is_gpu: isGPU,
      // Clear GPU fields if not GPU
      ...(isGPU ? {} : { gpu_type: undefined, gpu_count: undefined, gpu_memory_gib: undefined })
    }))
    setInstanceSearch(machine.id)
    setShowDropdown(false)
    setErrors({})
  }

  const presets = [
    { label: 'ubuntu-latest-4-cores', provider: 'github', instance_type: 'ubuntu-latest-4-cores', vcpus: 4, memory_gib: 16, cost_per_hour: 0.032 },
    { label: 'c7i.xlarge', provider: 'aws', instance_type: 'c7i.xlarge', vcpus: 4, memory_gib: 8, cost_per_hour: 0.178 },
    { label: 'n2-standard-4', provider: 'gcp', instance_type: 'n2-standard-4', vcpus: 4, memory_gib: 16, cost_per_hour: 0.194 },
  ]

  const applyPreset = (preset: typeof presets[0]) => {
    setForm({ ...preset, repository: '*' })
    setInstanceSearch(preset.instance_type)
    setShowForm(true)
    setErrors({})
  }

  const handleEdit = (m: LabelMapping) => {
    setForm(m)
    setInstanceSearch(m.instance_type)
    setEditingId(m.id)
    setShowForm(true)
    setErrors({})
  }

  const validate = (): boolean => {
    const e: Record<string, string> = {}
    if (!form.label?.trim()) e.label = 'Required'
    if (!form.provider) e.provider = 'Required'
    if (!form.instance_type?.trim()) e.instance_type = 'Required'
    if (!form.vcpus || form.vcpus < 1) e.vcpus = 'Min 1'
    if (!form.memory_gib || form.memory_gib < 0.5) e.memory_gib = 'Min 0.5'
    if (!form.cost_per_hour || form.cost_per_hour <= 0) e.cost_per_hour = 'Required'
    if (form.is_gpu && !form.gpu_type) e.gpu_type = 'Required'
    setErrors(e)
    return Object.keys(e).length === 0
  }

  const handleSubmit = async () => {
    if (!validate()) return
    setSaving(true)
    try {
      await upsertLabelMapping(form)
      setShowForm(false)
      setEditingId(null)
      setForm({ repository: '*', is_gpu: false, gpu_count: 1 })
      setInstanceSearch('')
      onRefresh()
    } catch (err) {
      console.error(err)
    }
    setSaving(false)
  }

  const handleCancel = () => {
    setShowForm(false)
    setEditingId(null)
    setForm({ repository: '*', is_gpu: false, gpu_count: 1 })
    setInstanceSearch('')
    setErrors({})
  }

  const handleDelete = async (id: string) => {
    if (confirm('Delete this mapping?')) {
      await deleteLabelMapping(id)
      onRefresh()
    }
  }

  return (
    <div>
      {/* Info banner */}
      <div className="mb-5 p-4 bg-[var(--cream-alt)] border-l-3 border-l-[var(--gold)]">
        <p className="text-sm text-[var(--text-mid)] leading-relaxed">
          <strong className="text-[var(--text)]">Label mappings</strong> define the specs and cost of your CI runners. 
          Map labels like <code>ubuntu-latest-16-cores</code> to their resources so RunRight can detect over-provisioned jobs.
        </p>
      </div>

      {/* Actions */}
      {!showForm && (
        <div className="mb-5">
          <div className="flex justify-between items-center mb-3">
            <h2 className="font-serif text-lg text-[var(--text)]">Quick Add Presets</h2>
            <button 
              className="btn-rr"
              onClick={() => { setShowForm(true); setEditingId(null); }}
            >
              + Custom Mapping
            </button>
          </div>
          <div className="flex flex-wrap gap-2 mb-3">
            {presets.map((p) => {
              const exists = mappings.some(m => m.label === p.label && m.repository === '*')
              return (
                <button
                  key={p.label}
                  onClick={() => !exists && applyPreset(p)}
                  disabled={exists}
                  className={`
                    px-3 py-1.5 text-xs font-mono border transition-colors cursor-pointer
                    ${exists 
                      ? 'border-[var(--border)] text-[var(--text-light)] bg-[var(--cream-alt)] cursor-not-allowed opacity-50' 
                      : 'border-[var(--border)] text-[var(--text-mid)] bg-[var(--paper)] hover:border-[var(--border-dark)] hover:text-[var(--text)]'}
                  `}
                  title={exists ? 'Already added' : `${p.vcpus} vCPU, ${p.memory_gib} GiB, $${p.cost_per_hour}/hr`}
                >
                  {p.label}
                </button>
              )
            })}
          </div>
        </div>
      )}

      {/* Form */}
      {showForm && (
        <div className="rr-card mb-5 !border-[var(--gold)]">
          <h3 className="font-serif text-lg text-[var(--text)] mb-4">
            {editingId ? 'Edit Mapping' : 'New Label Mapping'}
          </h3>
          
          {/* Row 1: Label + Scope + Provider */}
          <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-4">
            <div>
              <label className="block font-deco text-[11px] tracking-[1px] uppercase text-[var(--text-mid)] mb-1">
                Label <span className="text-[var(--red)]">*</span>
              </label>
              <input
                type="text"
                className={`rr-input ${errors.label ? '!border-[var(--red)]' : ''}`}
                value={form.label || ''}
                onChange={(e) => setForm({ ...form, label: e.target.value })}
                placeholder="ubuntu-latest-16-cores"
              />
            </div>
            <div className="relative" ref={repoDropdownRef}>
              <label className="block font-deco text-[11px] tracking-[1px] uppercase text-[var(--text-mid)] mb-1">
                Repository Scope
              </label>
              <input
                type="text"
                className="rr-input"
                value={form.repository === '*' ? '' : (repoSearch || form.repository || '')}
                onChange={(e) => {
                  setRepoSearch(e.target.value)
                  if (!e.target.value) setForm({ ...form, repository: '*' })
                }}
                onFocus={() => setShowRepoDropdown(true)}
                placeholder={form.repository === '*' ? 'All Repositories' : 'Search repos…'}
              />
              {showRepoDropdown && (
                <div className="absolute z-20 w-full mt-1 bg-[var(--paper)] border border-[var(--border)] shadow-md max-h-48 overflow-y-auto">
                  <button
                    type="button"
                    className={`w-full text-left px-3 py-2 text-sm hover:bg-[var(--cream)] flex items-center gap-2 ${form.repository === '*' ? 'bg-[var(--cream)]' : ''}`}
                    onClick={() => {
                      setForm({ ...form, repository: '*' })
                      setRepoSearch('')
                      setShowRepoDropdown(false)
                    }}
                  >
                    <span className="text-[var(--gold)]">★</span> All Repositories
                  </button>
                  {filteredRepos.map(r => (
                    <button
                      key={r.repository}
                      type="button"
                      className={`w-full text-left px-3 py-2 text-sm hover:bg-[var(--cream)] font-mono ${form.repository === r.repository ? 'bg-[var(--cream)]' : ''}`}
                      onClick={() => {
                        setForm({ ...form, repository: r.repository })
                        setRepoSearch('')
                        setShowRepoDropdown(false)
                      }}
                    >
                      {r.repository}
                    </button>
                  ))}
                  {filteredRepos.length === 0 && repoSearch && (
                    <div className="px-3 py-2 text-sm text-[var(--text-light)]">No matching repos</div>
                  )}
                </div>
              )}
            </div>
            <div>
              <label className="block font-deco text-[11px] tracking-[1px] uppercase text-[var(--text-mid)] mb-1">
                Provider <span className="text-[var(--red)]">*</span>
              </label>
              <select
                className={`rr-select w-full ${errors.provider ? '!border-[var(--red)]' : ''}`}
                value={form.provider || ''}
                onChange={(e) => {
                  setForm({ ...form, provider: e.target.value })
                  setInstanceSearch('')
                }}
              >
                <option value="">Select…</option>
                <option value="github">GitHub Actions</option>
                <option value="aws">AWS</option>
                <option value="gcp">GCP</option>
                <option value="azure">Azure</option>
                <option value="other">Other / Self-hosted</option>
              </select>
            </div>
          </div>

          {/* Row 2: Instance Type with Autocomplete */}
          {(() => {
            const providerHasCatalog = form.provider && ['aws', 'gcp', 'github', 'azure'].includes(form.provider)
            const catalogCount = providerHasCatalog ? filteredCatalog.length : 0
            const noResults = providerHasCatalog && instanceSearch.trim() && filteredCatalog.length === 0

            return (
              <div className="mb-4">
                <label className="block font-deco text-[11px] tracking-[1px] uppercase text-[var(--text-mid)] mb-1">
                  Instance Type <span className="text-[var(--red)]">*</span>
                  {providerHasCatalog && !instanceSearch && (
                    <span className="text-[var(--text-light)] normal-case tracking-normal ml-2">
                      — {catalog.filter(c => c.provider === form.provider).length} instances in catalog
                    </span>
                  )}
                </label>
                <div className="relative" ref={dropdownRef}>
                  <input
                    type="text"
                    className={`rr-input ${errors.instance_type ? '!border-[var(--red)]' : ''}`}
                    value={instanceSearch}
                    onChange={(e) => {
                      setInstanceSearch(e.target.value)
                      setForm({ ...form, instance_type: e.target.value })
                      setShowDropdown(true)
                    }}
                    onFocus={() => providerHasCatalog && setShowDropdown(true)}
                    placeholder={
                      !form.provider ? 'Select provider first' :
                      providerHasCatalog ? `Search ${form.provider.toUpperCase()} instances…` :
                      'Enter instance type (e.g. Standard_D4s_v3)'
                    }
                  />
                  
                  {/* Autocomplete Dropdown */}
                  {showDropdown && providerHasCatalog && (
                    <div className="absolute z-50 top-full left-0 right-0 mt-1 max-h-64 overflow-y-auto bg-[var(--paper)] border border-[var(--border)] shadow-lg">
                      {filteredCatalog.map((m) => {
                        const isGPU = machineHasGPU(m)
                        return (
                          <button
                            key={m.id}
                            type="button"
                            className="w-full px-3 py-2 text-left hover:bg-[var(--cream-alt)] border-b border-[var(--border)] last:border-b-0 cursor-pointer flex items-center justify-between"
                            onClick={() => applyMachineType(m)}
                          >
                            <div>
                              <code className="text-sm text-[var(--text)]">{m.id}</code>
                              <span className="text-[var(--text-light)] text-xs ml-2">
                                {m.vcpus} vCPU · {m.memory_gib} GiB
                              </span>
                            </div>
                            <div className="flex items-center gap-2">
                              {isGPU && <span className="badge badge-aws text-[10px]">GPU</span>}
                              <span className="text-[var(--text-light)] text-xs font-mono">${m.on_demand_price_per_hour.toFixed(3)}/hr</span>
                            </div>
                          </button>
                        )
                      })}
                      {noResults && (
                        <div className="px-3 py-3 text-sm text-[var(--text-light)] border-t border-[var(--border)] bg-[var(--cream-alt)]">
                          No matches for "<code>{instanceSearch}</code>". 
                          <span className="block mt-1 text-xs">Fill in specs manually below, or try: c7i (AWS), n2 (GCP), D4s_v5 (Azure)</span>
                        </div>
                      )}
                    </div>
                  )}
                </div>
                
                {/* Manual entry hint for unsupported providers */}
                {form.provider && !providerHasCatalog && (
                  <p className="text-xs text-[var(--text-light)] mt-1">
                    No catalog data for {form.provider === 'other' ? 'custom providers' : form.provider.toUpperCase()}. Enter instance details manually below.
                  </p>
                )}
              </div>
            )
          })()}

          {/* Row 3: Specs (auto-filled but editable) */}
          <div className="grid grid-cols-2 sm:grid-cols-5 gap-4 mb-4 p-3 bg-[var(--cream-alt)]">
            <div>
              <label className="block font-deco text-[10px] tracking-[1px] uppercase text-[var(--text-light)] mb-1">
                vCPUs {errors.vcpus && <span className="text-[var(--red)]">*</span>}
              </label>
              <input
                type="number"
                min="1"
                className={`rr-input !bg-[var(--paper)] ${errors.vcpus ? '!border-[var(--red)]' : ''}`}
                value={form.vcpus || ''}
                onChange={(e) => setForm({ ...form, vcpus: parseInt(e.target.value) || undefined })}
              />
            </div>
            <div>
              <label className="block font-deco text-[10px] tracking-[1px] uppercase text-[var(--text-light)] mb-1">
                Memory (GiB) {errors.memory_gib && <span className="text-[var(--red)]">*</span>}
              </label>
              <input
                type="number"
                min="0.5"
                step="0.5"
                className={`rr-input !bg-[var(--paper)] ${errors.memory_gib ? '!border-[var(--red)]' : ''}`}
                value={form.memory_gib || ''}
                onChange={(e) => setForm({ ...form, memory_gib: parseFloat(e.target.value) || undefined })}
              />
            </div>
            <div>
              <label className="block font-deco text-[10px] tracking-[1px] uppercase text-[var(--text-light)] mb-1">
                Cost/Hour ($) {errors.cost_per_hour && <span className="text-[var(--red)]">*</span>}
              </label>
              <input
                type="number"
                min="0.001"
                step="0.001"
                className={`rr-input !bg-[var(--paper)] ${errors.cost_per_hour ? '!border-[var(--red)]' : ''}`}
                value={form.cost_per_hour || ''}
                onChange={(e) => setForm({ ...form, cost_per_hour: parseFloat(e.target.value) || undefined })}
              />
            </div>

          </div>

          {/* GPU Config (only if is_gpu) */}
          {form.is_gpu && (
            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 mb-4 p-4 bg-[rgba(184,134,11,.06)] border border-[var(--gold)]">
              <div>
                <label className="block font-deco text-[11px] tracking-[1px] uppercase text-[var(--text-mid)] mb-1">
                  GPU Type <span className="text-[var(--red)]">*</span>
                </label>
                <select
                  className={`rr-select w-full ${errors.gpu_type ? '!border-[var(--red)]' : ''}`}
                  value={form.gpu_type || ''}
                  onChange={(e) => {
                    const tier = gpuTiers.find(t => t.type === e.target.value)
                    setForm({ ...form, gpu_type: e.target.value, gpu_memory_gib: tier?.memory_gib || form.gpu_memory_gib })
                  }}
                >
                  <option value="">Select GPU…</option>
                  {gpuTiers.map((t) => (
                    <option key={t.type} value={t.type}>{t.name} ({t.memory_gib} GB)</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block font-deco text-[11px] tracking-[1px] uppercase text-[var(--text-mid)] mb-1">GPU Count</label>
                <input
                  type="number"
                  min="1"
                  max="8"
                  className="rr-input"
                  value={form.gpu_count || 1}
                  onChange={(e) => setForm({ ...form, gpu_count: parseInt(e.target.value) || 1 })}
                />
              </div>
              <div>
                <label className="block font-deco text-[11px] tracking-[1px] uppercase text-[var(--text-mid)] mb-1">GPU Memory (GiB)</label>
                <input
                  type="number"
                  className="rr-input"
                  value={form.gpu_memory_gib || ''}
                  onChange={(e) => setForm({ ...form, gpu_memory_gib: parseFloat(e.target.value) || undefined })}
                />
              </div>
            </div>
          )}

          <div className="flex gap-3">
            <button className="btn-rr" onClick={handleSubmit} disabled={saving}>
              {saving ? 'Saving…' : editingId ? 'Update' : 'Add Mapping'}
            </button>
            <button 
              className="px-4 py-2 border border-[var(--border)] text-[var(--text-mid)] font-deco text-[13px] tracking-[1px] hover:border-[var(--border-dark)] hover:text-[var(--text)] transition-colors cursor-pointer bg-transparent"
              onClick={handleCancel}
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Table */}
      <div>
        <div className="flex items-center justify-between mb-4">
          <h2 className="font-serif text-lg text-[var(--text)]">
            Your Mappings
            <span className="ml-2 text-[var(--text-light)] font-sans text-sm font-normal">({mappings.length})</span>
          </h2>
        </div>
        
        {mappings.length === 0 ? (
          <div className="rr-card text-center py-10">
            <h3 className="font-serif text-lg text-[var(--text)] mb-2">No label mappings configured</h3>
            <p className="text-[var(--text-light)] text-sm">Use the presets above or add a custom mapping to get started.</p>
          </div>
        ) : (
          <div className="space-y-4">
            <ListControls 
              pagination={mappingsPagination} 
              searchPlaceholder="Search mappings..." 
            />
            <div className="rr-card !p-0">
              <div className="overflow-x-auto">
                <table className="rr-table">
                  <thead>
                    <tr>
                      <th>Label</th>
                      <th>Scope</th>
                      <th>Provider</th>
                      <th>Instance</th>
                      <th className="text-right">vCPU</th>
                      <th className="text-right">Memory</th>
                      <th className="text-right">$/hr</th>
                      <th>GPU</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {mappingsPagination.paginatedItems.map((m) => (
                    <tr key={m.id}>
                      <td><code className="text-[13px]">{m.label}</code></td>
                      <td>
                        {m.repository === '*' 
                          ? <span className="text-[#2E7D32] text-xs">All repos</span>
                          : <span className="text-xs">{m.repository}</span>}
                      </td>
                      <td><span className={`badge badge-${m.provider}`}>{m.provider.toUpperCase()}</span></td>
                      <td><code className="text-[12px]">{m.instance_type}</code></td>
                      <td className="text-right font-mono">{m.vcpus}</td>
                      <td className="text-right font-mono">{m.memory_gib}</td>
                      <td className="text-right font-mono">${m.cost_per_hour.toFixed(3)}</td>
                      <td>
                        {m.is_gpu 
                          ? <span className="badge badge-aws">{m.gpu_type?.replace('nvidia-', '').toUpperCase()}</span>
                          : <span className="text-[var(--text-light)]">—</span>}
                      </td>
                      <td>
                        <div className="flex gap-1">
                          {/* Pencil / edit */}
                          <button
                            onClick={() => handleEdit(m)}
                            className="p-1.5 rounded hover:bg-[var(--paper)] text-[var(--text-mid)] hover:text-[var(--text)] transition-colors"
                            title="Edit"
                          >
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none"
                              stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                              <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
                              <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>
                            </svg>
                          </button>
                          {/* Trash / delete */}
                          <button
                            onClick={() => handleDelete(m.id)}
                            className="p-1.5 rounded hover:bg-[var(--paper)] text-[var(--text-mid)] hover:text-[var(--red)] transition-colors"
                            title="Delete"
                          >
                            <svg width="14" height="14" viewBox="0 0 24 24" fill="none"
                              stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                              <polyline points="3 6 5 6 21 6"/>
                              <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>
                              <path d="M10 11v6M14 11v6"/>
                              <path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2"/>
                            </svg>
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>
        )}
      </div>
    </div>
  )
}

function SettingsTab({ settings, onSave }: {
  settings: AutoPRSettings
  onSave: (s: AutoPRSettings) => void
}) {
  const [form, setForm] = useState(settings)
  const [saving, setSaving] = useState(false)
  const [scanning, setScanning] = useState(false)
  // newToken is typed by the user when they want to replace/set the PAT.
  // We keep it separate so we don't accidentally overwrite a saved token
  // with an empty string on every save.
  const [newToken, setNewToken] = useState('')
  const [tokenVisible, setTokenVisible] = useState(false)

  const handleSave = async () => {
    setSaving(true)
    const payload = { ...form }
    if (newToken) payload.github_token = newToken
    else delete payload.github_token // don't send blank — preserve existing
    await onSave(payload)
    setNewToken('')
    setSaving(false)
  }

  return (
    <div className="max-w-2xl">
      {/* ── GitHub Token ─────────────────────────────────────────── */}
      <div className="rr-card mb-5">
        <h3 className="font-serif text-lg text-[var(--text)] mb-1">GitHub Token</h3>
        <p className="text-[var(--text-light)] text-sm mb-4">
          A Personal Access Token with <code className="text-xs bg-[var(--paper)] px-1 rounded">contents</code> and{' '}
          <code className="text-xs bg-[var(--paper)] px-1 rounded">pull_requests</code> scopes.
          Required to open PRs. Leave blank to use the server&apos;s{' '}
          <code className="text-xs bg-[var(--paper)] px-1 rounded">GITHUB_TOKEN</code> env var.
        </p>

        {/* Status pill — shown when a token is already stored */}
        {settings.github_token_set && !newToken && (
          <div className="flex items-center gap-3 mb-3 px-3 py-2 rounded bg-[var(--paper)] border border-[color:var(--border)]">
            {/* Lock icon */}
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none"
              stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
              className="text-green-500 shrink-0">
              <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/>
              <path d="M7 11V7a5 5 0 0 1 10 0v4"/>
            </svg>
            <span className="text-sm text-[var(--text)]">
              Token stored&ensp;<span className="font-mono text-xs text-[var(--text-light)]">{settings.github_token_hint}</span>
            </span>
            <button
              type="button"
              className="ml-auto text-xs text-[var(--red)] hover:underline"
              onClick={() => setNewToken(' ')} // trigger input reveal
            >
              Replace
            </button>
          </div>
        )}

        {/* Input — shown when no token or user clicked Replace */}
        {(!settings.github_token_set || newToken) && (
          <div className="flex gap-2">
            <input
              type={tokenVisible ? 'text' : 'password'}
              className="rr-input flex-1 font-mono text-sm"
              placeholder="ghp_…"
              value={newToken.trim() === '' ? '' : newToken}
              onChange={(e) => setNewToken(e.target.value)}
              autoComplete="off"
              // eslint-disable-next-line jsx-a11y/no-autofocus
              autoFocus={!!settings.github_token_set}
            />
            <button
              type="button"
              className="btn-rr-outline px-3"
              onClick={() => setTokenVisible((v) => !v)}
            >
              {tokenVisible ? 'Hide' : 'Show'}
            </button>
            {settings.github_token_set && (
              <button type="button" className="btn-rr-outline px-3"
                onClick={() => setNewToken('')}>Cancel</button>
            )}
          </div>
        )}
      </div>

      {/* ── Auto-PR Generation ────────────────────────────────────── */}
      <div className="rr-card mb-5">
        <h3 className="font-serif text-lg text-[var(--text)] mb-4">Auto-PR Generation</h3>
        <label className="flex items-center gap-3 cursor-pointer mb-3">
          <input
            type="checkbox"
            checked={form.enabled}
            onChange={(e) => setForm({ ...form, enabled: e.target.checked })}
            className="w-5 h-5 accent-[var(--red)]"
          />
          <span className="text-[var(--text)]">Enable automatic PR creation</span>
        </label>
        <p className="text-[var(--text-light)] text-sm">
          When enabled, PRs will be automatically created for approved recommendations.
        </p>
      </div>

      <div className="rr-card mb-5">
        <h3 className="font-serif text-lg text-[var(--text)] mb-4">Thresholds</h3>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label className="block font-deco text-[11px] tracking-[1px] uppercase text-[var(--text-mid)] mb-1">
              Min Savings %
            </label>
            <input
              type="number"
              className="rr-input"
              value={form.min_savings_percent}
              onChange={(e) => setForm({ ...form, min_savings_percent: parseFloat(e.target.value) })}
            />
          </div>
          <div>
            <label className="block font-deco text-[11px] tracking-[1px] uppercase text-[var(--text-mid)] mb-1">
              Consecutive Runs
            </label>
            <input
              type="number"
              className="rr-input"
              value={form.require_consecutive_runs}
              onChange={(e) => setForm({ ...form, require_consecutive_runs: parseInt(e.target.value) })}
            />
          </div>
          <div>
            <label className="block font-deco text-[11px] tracking-[1px] uppercase text-[var(--text-mid)] mb-1">
              Min Data Span (days)
            </label>
            <input
              type="number"
              min={0}
              className="rr-input"
              value={form.min_data_days}
              onChange={(e) => setForm({ ...form, min_data_days: parseInt(e.target.value) })}
            />
            <p className="text-[var(--text-light)] text-xs mt-1">
              Qualifying runs must span at least this many calendar days. Prevents incident-day spikes from triggering false recommendations. 0 = disabled.
            </p>
          </div>
          <div>
            <label className="block font-deco text-[11px] tracking-[1px] uppercase text-[var(--text-mid)] mb-1">
              Max Recs Per Scan
            </label>
            <input
              type="number"
              min={1}
              className="rr-input"
              value={form.max_recs_per_scan}
              onChange={(e) => setForm({ ...form, max_recs_per_scan: parseInt(e.target.value) })}
            />
            <p className="text-[var(--text-light)] text-xs mt-1">
              Hard cap on new recommendations created per background scan cycle.
            </p>
          </div>
        </div>
      </div>

      <div className="flex gap-3 flex-wrap">
        <button className="btn-rr" onClick={handleSave} disabled={saving}>
          {saving ? 'Saving…' : 'Save Settings'}
        </button>
        <button
          className="btn-rr-outline"
          disabled={scanning}
          onClick={async () => {
            setScanning(true)
            try { await triggerAutoPRScan() } catch { /* best-effort */ }
            setTimeout(() => setScanning(false), 3000)
          }}
          title="Re-scan all job history (up to 30 days) and surface new recommendations"
        >
          {scanning ? 'Scanning…' : 'Scan Now'}
        </button>
      </div>
    </div>
  )
}

function HistoryTab({ history }: { history: PRHistory[] }) {
  const pagination = usePagination({
    items: history,
    pageSize: 10,
    searchFn: (item, query) =>
      item.repository.toLowerCase().includes(query) ||
      item.job_id.toLowerCase().includes(query) ||
      item.old_label.toLowerCase().includes(query) ||
      item.new_label.toLowerCase().includes(query) ||
      item.status.toLowerCase().includes(query),
  })

  if (history.length === 0) {
    return (
      <div className="rr-card text-center py-12">
        <h3 className="font-serif text-lg text-[var(--text)] mb-2">No PR history</h3>
        <p className="text-[var(--text-light)] text-sm">When PRs are created, they will appear here.</p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <ListControls 
        pagination={pagination} 
        searchPlaceholder="Search history..." 
      />
      <div className="rr-card !p-0">
        <div className="overflow-x-auto">
          <table className="rr-table">
            <thead>
              <tr>
                <th>Repository</th>
                <th>Job</th>
                <th>PR</th>
                <th>Change</th>
                <th>Savings</th>
                <th>Status</th>
                <th>Created</th>
              </tr>
            </thead>
            <tbody>
              {pagination.paginatedItems.map((h) => (
                <tr key={h.id}>
                  <td className="font-mono text-xs">{h.repository}</td>
                  <td>{h.job_id}</td>
                  <td>
                    <a href={h.pr_url} target="_blank" rel="noopener noreferrer" className="link-btn">
                      #{h.pr_number}
                    </a>
                  </td>
                  <td>
                    <code className="text-xs">{h.old_label}</code>
                    <span className="mx-1 text-[var(--gold)]">→</span>
                    <code className="text-xs text-[#2E7D32]">{h.new_label}</code>
                  </td>
                  <td className="text-[#2E7D32] font-mono">
                    -{h.savings_percent.toFixed(0)}%
                  </td>
                  <td>
                    <span className={`badge ${h.status === 'merged' ? 'badge-right-sized' : h.status === 'closed' ? '!text-[var(--red)] !border-[var(--red)]' : ''}`}>
                      {h.status}
                    </span>
                  </td>
                  <td className="text-[var(--text-light)] text-xs">
                    {new Date(h.created_at).toLocaleDateString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  )
}
