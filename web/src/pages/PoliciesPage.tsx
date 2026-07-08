import { useEffect, useState } from 'react'
import { deletePolicy, fetchPolicies, fetchRepoJobs, fetchRepos, upsertPolicy, fetchUserSettings, upsertUserSettings, type UserSettings } from '../api'
import { convertFromUSD, convertToUSD, useCurrencyPreference } from '../currency'
import type { JobSummaryRow, PolicyRule, RepoSummary } from '../types'
import { useUser } from '../App'
import { ConfirmModal } from '../components/ConfirmModal'

export default function PoliciesPage() {
  const { can } = useUser()
  const canWrite = can('policies:manage')
  const { currency } = useCurrencyPreference()
  const [confirmModal, setConfirmModal] = useState<{ title: string; message: string; onConfirm: () => void } | null>(null)
  const [policyRepository, setPolicyRepository] = useState('')
  const [policyJobId, setPolicyJobId] = useState('')
  const [policyMaxCostPerHour, setPolicyMaxCostPerHour] = useState('')
  const [policyInputCurrency, setPolicyInputCurrency] = useState(currency)
  const [policies, setPolicies] = useState<PolicyRule[]>([])
  const [policyBusy, setPolicyBusy] = useState(false)
  const [policyError, setPolicyError] = useState('')
  const [policySaved, setPolicySaved] = useState(false)
  const [repoFilter, setRepoFilter] = useState('')
  const [repos, setRepos] = useState<RepoSummary[]>([])
  const [repoJobs, setRepoJobs] = useState<JobSummaryRow[]>([])
  const [activeTab, setActiveTab] = useState<'create' | 'list' | 'pool'>('create')

  // Pool constraints (persisted on backend)
  const [userSettings, setUserSettings] = useState<UserSettings>({
    otel_endpoint: '',
    allowed_machine_ids: [],
    allowed_series: [],
    allowed_families: [],
  })
  const [poolConstraints, setPoolConstraints] = useState({
    allowedMachineIDs: '',
    allowedSeries: '',
    allowedFamilies: '',
  })
  const [poolSaved, setPoolSaved] = useState(false)
  const [poolLoading, setPoolLoading] = useState(true)
  const [showPolicyRepoSuggestions, setShowPolicyRepoSuggestions] = useState(false)
  const [showPolicyJobSuggestions, setShowPolicyJobSuggestions] = useState(false)
  const [showFilterRepoSuggestions, setShowFilterRepoSuggestions] = useState(false)

  useEffect(() => {
    void (async () => {
      try {
        const settings = await fetchUserSettings()
        setUserSettings(settings)
        setPoolConstraints({
          allowedMachineIDs: settings.allowed_machine_ids.join(', '),
          allowedSeries: settings.allowed_series.join(', '),
          allowedFamilies: settings.allowed_families.join(', '),
        })
      } catch {
        // Failed to load, defaults are fine
      } finally {
        setPoolLoading(false)
      }
    })()
  }, [])

  useEffect(() => {
    void refreshPolicies()
    void fetchRepos().then(setRepos).catch(() => {})
  }, [])

  const repoSuggestions = (query: string) =>
    (() => {
      const uniqueRepos = Array.from(new Set(repos.map((r) => r.repository).filter(Boolean)))
      const normalizedQuery = query.trim().toLowerCase()
      if (!normalizedQuery) return uniqueRepos.slice(0, 8)
      const matches = uniqueRepos.filter((repo) => repo.toLowerCase().includes(normalizedQuery))
      const exactMatch = uniqueRepos.some((repo) => repo.toLowerCase() === normalizedQuery)
      // If current input is an exact picked value, keep list broad so reselection is easy.
      if (exactMatch) return uniqueRepos.slice(0, 8)
      return matches.slice(0, 8)
    })()

  const jobSuggestions = (query: string) =>
    (() => {
      const uniqueJobs = Array.from(new Set(repoJobs.map((j) => j.job_id).filter(Boolean)))
      const normalizedQuery = query.trim().toLowerCase()
      if (!normalizedQuery) return uniqueJobs.slice(0, 8)
      const matches = uniqueJobs.filter((job) => job.toLowerCase().includes(normalizedQuery))
      const exactMatch = uniqueJobs.some((job) => job.toLowerCase() === normalizedQuery)
      // If current input is an exact picked value, keep list broad so reselection is easy.
      if (exactMatch) return uniqueJobs.slice(0, 8)
      return matches.slice(0, 8)
    })()

  const policyRepoMatches = repoSuggestions(policyRepository)
  const filterRepoMatches = repoSuggestions(repoFilter)
  const policyJobMatches = jobSuggestions(policyJobId)

  useEffect(() => {
    const repository = policyRepository.trim()
    if (!repository) {
      setRepoJobs([])
      return
    }
    void fetchRepoJobs(repository).then(setRepoJobs).catch(() => setRepoJobs([]))
  }, [policyRepository])

  useEffect(() => {
    if (policyInputCurrency === currency) return
    setPolicyMaxCostPerHour((prev) => {
      const parsed = Number(prev)
      if (!Number.isFinite(parsed) || parsed <= 0) return prev
      const usd = convertToUSD(parsed, policyInputCurrency)
      const digits = currency === 'JPY' ? 0 : 2
      return convertFromUSD(usd, currency).toFixed(digits)
    })
    setPolicyInputCurrency(currency)
  }, [currency, policyInputCurrency])

  async function refreshPolicies(repository?: string) {
    try {
      setPolicyError('')
      setPolicies(await fetchPolicies(repository))
    } catch (e) {
      const errorMsg = e instanceof Error ? e.message : String(e)
      setPolicyError(errorMsg.includes('404') ? 'Failed to fetch policies. Check that the backend is running and accessible.' : errorMsg)
    }
  }

  async function savePolicy(e: React.FormEvent) {
    e.preventDefault()
    setPolicyBusy(true)
    setPolicyError('')
    setPolicySaved(false)

    const trimmedRepository = policyRepository.trim()
    const trimmedJobId = policyJobId.trim()
    const parsedMax = Number(policyMaxCostPerHour)
    const parsedMaxUSD = convertToUSD(parsedMax, currency)

    if (trimmedJobId && !trimmedRepository) {
      setPolicyError('Repository is required when targeting a specific job.')
      setPolicyBusy(false)
      return
    }
    if (!Number.isFinite(parsedMaxUSD) || parsedMaxUSD <= 0) {
      setPolicyError('Max cost per hour must be a number greater than zero.')
      setPolicyBusy(false)
      return
    }

    try {
      await upsertPolicy({
        repository: trimmedRepository,
        job_id: trimmedJobId || undefined,
        max_cost_per_hour: parsedMaxUSD,
        enabled: true,
      })
      setPolicyRepository('')
      setPolicyJobId('')
      setPolicyMaxCostPerHour('')
      setPolicySaved(true)
      await refreshPolicies(repoFilter.trim() || undefined)
      setTimeout(() => setPolicySaved(false), 2500)
    } catch (e) {
      setPolicyError(String(e))
    } finally {
      setPolicyBusy(false)
    }
  }

  async function removePolicy(rule: PolicyRule) {
    setPolicyBusy(true)
    setPolicyError('')
    try {
      await deletePolicy(rule.repository, rule.job_id)
      await refreshPolicies(repoFilter.trim() || undefined)
    } catch (e) {
      setPolicyError(String(e))
    } finally {
      setPolicyBusy(false)
    }
  }

  function confirmRemovePolicy(rule: PolicyRule) {
    const scope = rule.repository || 'global'
    const label = rule.job_id ? `${scope} / ${rule.job_id}` : scope
    setConfirmModal({
      title: 'Delete policy',
      message: `Permanently delete the cost policy for "${label}"? This cannot be undone.`,
      onConfirm: () => { setConfirmModal(null); void removePolicy(rule) },
    })
  }

  const policySnippet = `- uses: sgbudje/runright@v1
  with:
    run: make build
    export: file,http
    http-url: https://your-runright-backend.example.com`

  const backendEnforcementSnippet = `POST /api/v1/policies/evaluate
{
  "repository": "owner/repo",
  "job_id": "build",
  "detected_price_per_hour": 0.0832
}`

  function savePoolConstraints(e: React.FormEvent) {
    e.preventDefault()
    const updated: UserSettings = {
      ...userSettings,
      allowed_machine_ids: poolConstraints.allowedMachineIDs
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean),
      allowed_series: poolConstraints.allowedSeries
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean),
      allowed_families: poolConstraints.allowedFamilies
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean),
    }
    setPolicyBusy(true)
    void upsertUserSettings(updated)
      .then(() => {
        setUserSettings(updated)
        setPoolSaved(true)
        setTimeout(() => setPoolSaved(false), 2500)
      })
      .catch(() => {
        setPolicyError('Failed to save pool constraints.')
      })
      .finally(() => setPolicyBusy(false))
  }

  function clearPoolConstraints() {
    const empty: UserSettings = {
      ...userSettings,
      allowed_machine_ids: [],
      allowed_series: [],
      allowed_families: [],
    }
    setPoolConstraints({ allowedMachineIDs: '', allowedSeries: '', allowedFamilies: '' })
    setPolicyBusy(true)
    void upsertUserSettings(empty)
      .then(() => {
        setUserSettings(empty)
      })
      .catch(() => {
        setPolicyError('Failed to clear pool constraints.')
      })
      .finally(() => setPolicyBusy(false))
  }

  const poolEnvSnippet = [
    poolConstraints.allowedMachineIDs ? `RUNRIGHT_ALLOWED_MACHINE_IDS: ${poolConstraints.allowedMachineIDs}` : null,
    poolConstraints.allowedSeries ? `RUNRIGHT_ALLOWED_SERIES: ${poolConstraints.allowedSeries}` : null,
    poolConstraints.allowedFamilies ? `RUNRIGHT_ALLOWED_FAMILIES: ${poolConstraints.allowedFamilies}` : null,
  ].filter(Boolean).join('\n') || '# No constraints saved yet'

  const tabBtn = (tab: typeof activeTab, label: string) => (
    <button
      type="button"
      onClick={() => setActiveTab(tab)}
      className={`px-5 py-2.5 font-deco text-[13px] tracking-[1.5px] uppercase border-b-2 transition-colors ${
        activeTab === tab
          ? 'border-[var(--red)] text-[var(--text)]'
          : 'border-transparent text-[var(--text-light)] hover:text-[var(--text-mid)]'
      }`}
    >{label}</button>
  )

  return (
    <>
    <div className="fadein max-w-[1200px]">
      <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] mb-7 tracking-tight">Policies</h1>
      <div className="rr-card">
        <p className="text-sm text-[var(--text-light)] leading-relaxed">
          Policies are cost guardrails for CI jobs. Define a max $/hour, then have CI enforce it automatically before expensive runner choices become ongoing waste.
        </p>
        <div className="flex border-b border-[var(--border)] mb-6 mt-4">
          {tabBtn('create', 'Create Policy')}
          {tabBtn('list', 'Policy List')}
          {tabBtn('pool', 'Pool Constraints')}
        </div>

        {activeTab === 'create' && (
          <>
            <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mt-4 mb-5">
              <div className="bg-paper border border-[var(--border)] rounded px-3 py-2.5">
                <div className="font-deco text-[11px] tracking-[1.5px] text-[var(--text-mid)] uppercase mb-1">Global</div>
                <div className="text-xs text-[var(--text-light)]">Default limit for every repository and job.</div>
              </div>
              <div className="bg-paper border border-[var(--border)] rounded px-3 py-2.5">
                <div className="font-deco text-[11px] tracking-[1.5px] text-[var(--text-mid)] uppercase mb-1">Repository</div>
                <div className="text-xs text-[var(--text-light)]">Override for one repo when its budget differs.</div>
              </div>
              <div className="bg-paper border border-[var(--border)] rounded px-3 py-2.5">
                <div className="font-deco text-[11px] tracking-[1.5px] text-[var(--text-mid)] uppercase mb-1">Job</div>
                <div className="text-xs text-[var(--text-light)]">Fine-tune strict limits for specific pipelines.</div>
              </div>
            </div>

            <div className="grid grid-cols-1 lg:grid-cols-2 gap-5 mt-4">
              <form className="settings-form max-w-none" onSubmit={savePolicy}>
                <div className="form-group">
                  <label>Repository</label>
                  <input
                    type="text"
                    placeholder="owner/repo"
                    value={policyRepository}
                    onChange={(e) => {
                      setPolicyRepository(e.target.value)
                      setPolicyJobId('')
                    }}
                    onFocus={() => setShowPolicyRepoSuggestions(true)}
                    onClick={() => setShowPolicyRepoSuggestions(true)}
                    onBlur={() => setTimeout(() => setShowPolicyRepoSuggestions(false), 120)}
                  />
                  {showPolicyRepoSuggestions && policyRepoMatches.length > 0 && (
                    <div className="mt-2 border border-[var(--border)] rounded max-h-28 overflow-auto bg-paper">
                      {policyRepoMatches.map((repo) => (
                        <button
                          key={repo}
                          type="button"
                          className="block w-full text-left px-2 py-1.5 text-xs hover:bg-[var(--cream-alt)]"
                          onMouseDown={() => {
                            setPolicyRepository(repo)
                            setPolicyJobId('')
                            setShowPolicyRepoSuggestions(false)
                            setShowPolicyJobSuggestions(false)
                          }}
                        >
                          {repo}
                        </button>
                      ))}
                    </div>
                  )}
                  <p className="text-xs text-[var(--text-light)] mt-1.5">Leave blank for a global default policy.</p>
                </div>
                <div className="form-group">
                  <label>Job ID</label>
                  <input
                    type="text"
                    placeholder="build"
                    value={policyJobId}
                    onChange={(e) => setPolicyJobId(e.target.value)}
                    onFocus={() => setShowPolicyJobSuggestions(true)}
                    onClick={() => setShowPolicyJobSuggestions(true)}
                    onBlur={() => setTimeout(() => setShowPolicyJobSuggestions(false), 120)}
                  />
                  {showPolicyJobSuggestions && policyRepository.trim() && policyJobMatches.length > 0 && (
                    <div className="mt-2 border border-[var(--border)] rounded max-h-28 overflow-auto bg-paper">
                      {policyJobMatches.map((jobId) => (
                        <button
                          key={jobId}
                          type="button"
                          className="block w-full text-left px-2 py-1.5 text-xs hover:bg-[var(--cream-alt)]"
                          onMouseDown={() => {
                            setPolicyJobId(jobId)
                            setShowPolicyJobSuggestions(false)
                          }}
                        >
                          {jobId}
                        </button>
                      ))}
                    </div>
                  )}
                  <p className="text-xs text-[var(--text-light)] mt-1.5">Leave blank to apply the policy to all jobs in the repository.</p>
                </div>
                <div className="form-group">
                  <label>Max Cost Per Hour ({currency}/hr)</label>
                  <input
                    type="number"
                    min="0"
                    step={currency === 'JPY' ? '1' : '0.01'}
                    placeholder={convertFromUSD(0.5, currency).toFixed(currency === 'JPY' ? 0 : 2)}
                    value={policyMaxCostPerHour}
                    onChange={(e) => setPolicyMaxCostPerHour(e.target.value)}
                  />
                  <p className="text-xs text-[var(--text-light)] mt-1.5">Saved and enforced as USD/hr internally; shown here in {currency} for easier entry.</p>
                </div>
                {policyError && <p className="text-red text-sm mb-3">{policyError}</p>}
                <div className="flex items-center gap-4 flex-wrap">
                  <button className="btn-rr" type="submit" disabled={policyBusy || !canWrite} title={!canWrite ? 'You do not have permission to manage policies' : undefined}>{policyBusy ? 'Saving…' : 'Save Policy'}</button>
                  {policySaved && <span className="font-deco text-[15px] tracking-[2px] text-[#2E7D32]">Saved</span>}
                </div>
              </form>

              <div>
                <h3 className="font-serif text-[16px] font-bold text-[var(--text)] mb-2">How CI enforces it</h3>
                <p className="text-sm text-[var(--text-light)] leading-relaxed">
                  When the action is given <code>http-url</code>, it can POST the detected machine price and repo/job identifiers to the backend. The backend returns the effective policy and whether it was violated.
                </p>
                <pre className="bg-[#1A0F02] border border-[#3a2510] border-l-[3px] border-l-gold px-4 py-4 mt-3 text-xs overflow-x-auto text-gold-light font-mono leading-loose">{policySnippet}</pre>
                <pre className="bg-[#1A0F02] border border-[#3a2510] border-l-[3px] border-l-gold px-4 py-4 mt-3 text-xs overflow-x-auto text-gold-light font-mono leading-loose">{backendEnforcementSnippet}</pre>
              </div>
            </div>
          </>
        )}

        {activeTab === 'list' && (
          <div className="mt-1">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between mb-3">
            <div>
              <h3 className="font-serif text-[16px] font-bold text-[var(--text)] mb-1">Existing policies</h3>
              <p className="text-xs text-[var(--text-light)]">Filter by repository to review a single repo’s rules.</p>
            </div>
            <div className="flex gap-2 items-end">
              <div>
                <label className="block font-deco text-[11px] tracking-[1.5px] text-[var(--text-mid)] uppercase mb-1.5">Repository Filter</label>
                <input
                  type="text"
                  placeholder="owner/repo"
                  value={repoFilter}
                  onChange={(e) => setRepoFilter(e.target.value)}
                  className="rr-input"
                  onFocus={() => setShowFilterRepoSuggestions(true)}
                  onBlur={() => setTimeout(() => setShowFilterRepoSuggestions(false), 120)}
                />
                {showFilterRepoSuggestions && filterRepoMatches.length > 0 && (
                  <div className="mt-2 border border-[var(--border)] rounded max-h-28 overflow-auto bg-paper">
                    {filterRepoMatches.map((repo) => (
                      <button
                        key={repo}
                        type="button"
                        className="block w-full text-left px-2 py-1.5 text-xs hover:bg-[var(--cream-alt)]"
                        onMouseDown={() => {
                          setRepoFilter(repo)
                          setShowFilterRepoSuggestions(false)
                        }}
                      >
                        {repo}
                      </button>
                    ))}
                  </div>
                )}
              </div>
              <button
                type="button"
                className="btn-rr"
                onClick={() => void refreshPolicies(repoFilter.trim() || undefined)}
              >
                Refresh
              </button>
            </div>
          </div>
          {policies.length === 0 ? (
            <div className="empty text-base">No policies configured yet.</div>
          ) : (
            <>
            <div className="sm:hidden space-y-3 mb-4">
              {policies.map((rule) => (
                <div key={`${rule.repository}::${rule.job_id}`} className="bg-paper border border-[var(--border)] rounded-lg px-4 py-3 shadow-rr">
                  <div className="font-sans font-semibold text-sm text-[var(--text)] break-all">
                    {rule.repository || 'Global'}{rule.job_id ? <span className="text-[var(--text-light)]"> / {rule.job_id}</span> : null}
                  </div>
                  <div className="flex flex-wrap gap-3 mt-2 text-xs text-[var(--text-mid)]">
                    <span>${rule.max_cost_per_hour.toFixed(4)}/hr</span>
                    <span>{rule.enabled ? 'Enabled' : 'Disabled'}</span>
                    <span>{new Date(rule.updated_at).toLocaleDateString()}</span>
                  </div>
                  <button
                    type="button"
                    onClick={() => confirmRemovePolicy(rule)}
                    className="mt-3 text-red underline underline-offset-2 text-xs"
                    disabled={policyBusy || !canWrite}
                    title={!canWrite ? 'You do not have permission to manage policies' : undefined}
                  >
                    Delete
                  </button>
                </div>
              ))}
            </div>

            <div className="table-wrap hidden sm:block">
              <table className="rr-table min-w-[760px]">
                <thead>
                  <tr>
                    <th>Scope</th>
                    <th>Max/hr</th>
                    <th>Enabled</th>
                    <th>Updated</th>
                    <th className="text-right">Action</th>
                  </tr>
                </thead>
                <tbody>
                  {policies.map((rule) => (
                    <tr key={`${rule.repository}::${rule.job_id}`}>
                      <td>
                        {rule.repository || 'Global'}
                        {rule.job_id ? <span className="text-[var(--text-light)]"> / {rule.job_id}</span> : null}
                      </td>
                      <td className="tabular-nums">${rule.max_cost_per_hour.toFixed(4)}</td>
                      <td>{rule.enabled ? 'Yes' : 'No'}</td>
                      <td className="tabular-nums">{new Date(rule.updated_at).toLocaleString()}</td>
                      <td className="text-right">
                        <button
                          type="button"
                          onClick={() => confirmRemovePolicy(rule)}
                          className="text-red underline underline-offset-2"
                          disabled={policyBusy || !canWrite}
                          title={!canWrite ? 'You do not have permission to manage policies' : undefined}
                        >
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            </>
          )}
          </div>
        )}
        {activeTab === 'pool' && (
          <div className="mt-1">
            {poolLoading ? (
              <p className="text-sm text-[var(--text-light)]">Loading pool constraints...</p>
            ) : (
              <>
                <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">
                  Restrict recommendations to the machines your runner pool actually has available (AWS, GCP, or mixed). RunRight filters candidates to the allowed set before ranking, and falls back to the closest pool option when headroom cannot be met.
                </p>

                <div className="grid grid-cols-1 lg:grid-cols-2 gap-5">
                  <form className="settings-form max-w-none" onSubmit={savePoolConstraints}>
                    <div className="form-group">
                      <label>Allowed Machine IDs</label>
                      <input
                        type="text"
                        placeholder="c7g.2xlarge, m7i.xlarge"
                        value={poolConstraints.allowedMachineIDs}
                        onChange={(e) => setPoolConstraints((p) => ({ ...p, allowedMachineIDs: e.target.value }))}
                      />
                      <p className="text-xs text-[var(--text-light)] mt-1.5">Comma-separated exact instance IDs. Most specific — use when your pool is a fixed named list.</p>
                    </div>
                <div className="form-group">
                  <label>Allowed Series</label>
                  <input
                    type="text"
                    placeholder="c7g, m7i"
                    value={poolConstraints.allowedSeries}
                    onChange={(e) => setPoolConstraints((p) => ({ ...p, allowedSeries: e.target.value }))}
                  />
                  <p className="text-xs text-[var(--text-light)] mt-1.5">Comma-separated series names. Matches all sizes in that series (e.g. <code>c7g</code> or <code>n2</code>).</p>
                </div>
                <div className="form-group">
                  <label>Allowed Families</label>
                  <input
                    type="text"
                    placeholder="c, m, r"
                    value={poolConstraints.allowedFamilies}
                    onChange={(e) => setPoolConstraints((p) => ({ ...p, allowedFamilies: e.target.value }))}
                  />
                  <p className="text-xs text-[var(--text-light)] mt-1.5">Comma-separated family prefixes. Broadest filter — <code>c</code> matches all compute-optimized series (c7g, c7i, c6g…).</p>
                </div>
                {(poolConstraints.allowedMachineIDs || poolConstraints.allowedSeries || poolConstraints.allowedFamilies) && (
                  <div className="bg-amber-50 border border-amber-200 rounded px-3 py-2.5 text-xs text-amber-800 mb-3">
                    Constraints are applied in order: IDs → Series → Families. The most specific match wins. If no constraint is set, the full catalog is used.
                  </div>
                )}
                <div className="flex items-center gap-4 flex-wrap">
                  <button className="btn-rr" type="submit">Save Constraints</button>
                  <button type="button" onClick={clearPoolConstraints} className="text-sm text-[var(--text-light)] underline underline-offset-2">Clear</button>
                  {poolSaved && <span className="font-deco text-[15px] tracking-[2px] text-[#2E7D32]">Saved</span>}
                </div>
              </form>

              <div>
                <h3 className="font-serif text-[16px] font-bold text-[var(--text)] mb-2">Use in CI</h3>
                <p className="text-sm text-[var(--text-light)] leading-relaxed mb-3">
                  Pass these as environment variables or flags to <code>runright monitor</code> and <code>runright recommend</code>.
                </p>
                <pre className="bg-[#1A0F02] border border-[#3a2510] border-l-[3px] border-l-gold px-4 py-4 text-xs overflow-x-auto text-gold-light font-mono leading-loose whitespace-pre-wrap">{poolEnvSnippet}</pre>
                <p className="text-xs text-[var(--text-light)] mt-3 leading-relaxed">
                  Or pass them as flags:<br />
                  <code>--allowed-machine-ids "c7g.2xlarge,m7i.xlarge,n2-standard-8"</code><br />
                  <code>--allowed-series "c7g,m7i,n2,e2"</code><br />
                  <code>--allowed-families "c,m,r,n,e"</code>
                </p>
                <div className="mt-4 bg-paper border border-[var(--border)] rounded px-3 py-2.5 text-xs text-[var(--text-mid)]">
                  <strong>Fallback behaviour:</strong> when no machine in the allowed pool can meet required headroom, RunRight returns the closest available pool option with a clear note explaining the pool was exhausted — so CI never silently picks an out-of-pool machine.
                </div>
              </div>
            </div>
              </>
            )}
          </div>
        )}
      </div>
    </div>
    {confirmModal && (
      <ConfirmModal
        open={true}
        title={confirmModal.title}
        message={confirmModal.message}
        confirmLabel="Delete"
        danger
        onConfirm={confirmModal.onConfirm}
        onCancel={() => setConfirmModal(null)}
      />
    )}
    </>
  )
}