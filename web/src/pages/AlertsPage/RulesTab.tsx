import { useEffect, useState } from 'react'
import { fetchRepos, fetchRepoJobs, fetchPolicies } from '../../api'
import { convertFromUSD, convertToUSD, useCurrencyPreference } from '../../currency'
import type { JobSummaryRow, PolicyRule, RepoSummary, NotificationSettings } from '../../types'
import type { AlertRule, EventRuleDraft, ThresholdRuleDraft, SlackDestination } from '../AlertsPage/types'
import { useUser } from '../../App'
import { ConfirmModal } from '../../components/ConfirmModal'
import {
  eventDescription,
  eventLabel,
  metricLabel,
  thresholdHint,
  thresholdPlaceholder,
  thresholdStep,
  thresholdUnit,
  convertThresholdForMetricSwitch,
  costInputDigits,
  displayCostFromUSD,
} from './utils'

export interface RulesTabProps {
  rules: AlertRule[]
  settings: NotificationSettings
  onRulesChange: (rules: AlertRule[]) => Promise<void>
  onError: (msg: string) => void
  onNote: (msg: string) => void
  onSwitchTab?: (tab: 'rules' | 'destinations' | 'ownership' | 'deliveries') => void
}

type RuleType = 'event' | 'threshold'
type EventType = 'policy_violation' | 'high_waste' | 'daily_summary'

export default function RulesTab({ rules, settings, onRulesChange, onError, onNote, onSwitchTab }: RulesTabProps) {
  const { can } = useUser()
  const [confirmModal, setConfirmModal] = useState<{ id: string; name: string } | null>(null)
  const { currency } = useCurrencyPreference()
  const [ruleType, setRuleType] = useState<RuleType>('threshold')
  const [editingRuleId, setEditingRuleId] = useState<string | null>(null)
  const [repos, setRepos] = useState<RepoSummary[]>([])
  const [repoJobs, setRepoJobs] = useState<JobSummaryRow[]>([])
  const [policies, setPolicies] = useState<PolicyRule[]>([])
  const [busy, setBusy] = useState(false)
  const [thresholdDraftCurrency, setThresholdDraftCurrency] = useState(currency)
  const [rulesSearchQuery, setRulesSearchQuery] = useState('')

  const [thresholdDraft, setThresholdDraft] = useState<ThresholdRuleDraft>({
    name: '',
    scope: 'global',
    repository: '',
    jobId: '',
    metric: 'max_cost_per_hour',
    threshold: displayCostFromUSD(0.5, currency),
    destinationIds: [],
  })

  const [eventDraft, setEventDraft] = useState<EventRuleDraft>({
    name: '',
    event: 'policy_violation',
    policyKey: '',
    scope: 'global',
    repository: '',
    jobId: '',
    destinationIds: [],
  })

  const [showEventRepoSuggestions, setShowEventRepoSuggestions] = useState(false)
  const [showEventJobSuggestions, setShowEventJobSuggestions] = useState(false)
  const [showThresholdRepoSuggestions, setShowThresholdRepoSuggestions] = useState(false)
  const [showThresholdJobSuggestions, setShowThresholdJobSuggestions] = useState(false)
  const [showPolicySuggestions, setShowPolicySuggestions] = useState(false)
  const [policySearch, setPolicySearch] = useState('')
  const [showDestinationPicker, setShowDestinationPicker] = useState(false)
  const [destinationSearch, setDestinationSearch] = useState('')

  // Load initial data
  useEffect(() => {
    void fetchRepos().then(setRepos).catch(() => setRepos([]))
    void fetchPolicies().then(setPolicies).catch(() => setPolicies([]))
    const firstId = settings.slack.destinations?.[0]?.id
    setThresholdDraft((prev) => ({ ...prev, destinationIds: firstId ? [firstId] : [] }))
    setEventDraft((prev) => ({ ...prev, destinationIds: firstId ? [firstId] : [] }))
  }, [])

  // Load jobs when repo changes
  useEffect(() => {
    const activeScope = ruleType === 'threshold' ? thresholdDraft.scope : eventDraft.scope
    const activeRepo = ruleType === 'threshold' ? thresholdDraft.repository : eventDraft.repository
    if (activeRepo && activeScope === 'job') {
      void fetchRepoJobs(activeRepo).then(setRepoJobs).catch(() => setRepoJobs([]))
    } else {
      setRepoJobs([])
    }
  }, [ruleType, thresholdDraft.scope, thresholdDraft.repository, eventDraft.scope, eventDraft.repository])

  // Handle currency changes
  useEffect(() => {
    if (thresholdDraftCurrency === currency) return
    setThresholdDraft((prev) => {
      if (prev.metric !== 'max_cost_per_hour') return prev
      const usdValue = convertFromUSD(Number(prev.threshold), thresholdDraftCurrency)
      const newDisplayValue = displayCostFromUSD(usdValue, currency)
      return { ...prev, threshold: newDisplayValue }
    })
    setThresholdDraftCurrency(currency)
  }, [currency, thresholdDraftCurrency])

  const repoSuggestions = (query: string) =>
    Array.from(new Set(repos.map((r) => r.repository).filter(Boolean)))
      .filter((repo) => query.trim() === '' || repo.toLowerCase().includes(query.toLowerCase()))
      .slice(0, 8)

  const jobSuggestions = (query: string) =>
    Array.from(new Set(repoJobs.map((j) => j.job_id).filter(Boolean)))
      .filter((job) => query.trim() === '' || job.toLowerCase().includes(query.toLowerCase()))
      .slice(0, 8)

  const policyOptions = policies
    .filter((p) => p.enabled)
    .map((p) => ({
      key: `${p.repository}::${p.job_id}`,
      label:
        p.repository && p.job_id ? `${p.repository} / ${p.job_id}` : p.repository ? `${p.repository} (repo default)` : 'Global policy',
      repository: p.repository,
      jobId: p.job_id,
    }))

  const filteredPolicyOptions = (query: string) => {
    const q = query.trim().toLowerCase()
    if (!q) return policyOptions.slice(0, 20)
    return policyOptions
      .filter(
        (p) =>
          p.label.toLowerCase().includes(q) ||
          p.repository.toLowerCase().includes(q) ||
          p.jobId.toLowerCase().includes(q)
      )
      .slice(0, 20)
  }

  // Sync policy search with selection
  useEffect(() => {
    if (!eventDraft.policyKey) {
      setPolicySearch('')
      return
    }
    const picked = policyOptions.find((p) => p.key === eventDraft.policyKey)
    setPolicySearch(picked?.label ?? '')
  }, [eventDraft.policyKey, policyOptions])

  const activeDestinationIds = ruleType === 'threshold' ? thresholdDraft.destinationIds : eventDraft.destinationIds
  const allDestinations = [
    ...settings.slack.destinations.map((d) => ({ id: d.id, name: d.name, type: 'Slack' as const })),
    ...settings.teams.destinations.map((d) => ({ id: d.id, name: d.name, type: 'Teams' as const })),
    ...(settings.webhooks?.destinations ?? []).map((d) => ({ id: d.id, name: d.name, type: 'Webhook' as const })),
  ]
  const selectedDestinations = allDestinations.filter((d) => activeDestinationIds.includes(d.id))

  const icons = {
    event: (
      <svg viewBox="0 0 20 20" aria-hidden="true" className="h-3.5 w-3.5">
        <path fill="currentColor" d="M10 1a1 1 0 0 1 1 1v1.07a6 6 0 0 1 4.93 5.9V12l1.46 2.2A1 1 0 0 1 16.56 16H3.44a1 1 0 0 1-.83-1.8L4.07 12V8.97A6 6 0 0 1 9 3.07V2a1 1 0 0 1 1-1Zm-2.5 16h5a2.5 2.5 0 0 1-5 0Z" />
      </svg>
    ),
    threshold: (
      <svg viewBox="0 0 20 20" aria-hidden="true" className="h-3.5 w-3.5">
        <path fill="currentColor" d="M2.5 14a1 1 0 0 0 0 2h15a1 1 0 1 0 0-2h-15ZM2.5 9a1 1 0 1 0 0 2h10a1 1 0 1 0 0-2h-10Zm0-5a1 1 0 1 0 0 2h6a1 1 0 1 0 0-2h-6Z" />
      </svg>
    ),
    edit: (
      <svg viewBox="0 0 20 20" aria-hidden="true" className="h-3.5 w-3.5">
        <path d="M13.8 3.2 16.8 6.2 7.2 15.8 3.6 16.6 4.4 13z" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    ),
    trash: (
      <svg viewBox="0 0 20 20" aria-hidden="true" className="h-3.5 w-3.5">
        <path d="M4.5 5.5h11" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
        <path d="M7.2 5.5V4.2c0-.5.4-.9.9-.9h3.8c.5 0 .9.4.9.9v1.3" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
        <path d="m6.2 7 1 9.2c.1.6.5 1 1.1 1h3.4c.6 0 1-.4 1.1-1l1-9.2" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    ),
  }

  function toggleRuleDestination(destId: string, checked: boolean) {
    if (ruleType === 'threshold') {
      setThresholdDraft((prev) =>
        checked
          ? { ...prev, destinationIds: [...prev.destinationIds, destId] }
          : { ...prev, destinationIds: prev.destinationIds.filter((id) => id !== destId) }
      )
    } else {
      setEventDraft((prev) =>
        checked
          ? { ...prev, destinationIds: [...prev.destinationIds, destId] }
          : { ...prev, destinationIds: prev.destinationIds.filter((id) => id !== destId) }
      )
    }
  }

  const destinationNames = (ids: string[]): string =>
    ids.map((id) => allDestinations.find((d) => d.id === id)?.name ?? id).join(', ') || '—'

  async function addRule(e: React.FormEvent) {
    e.preventDefault()
    const name = (ruleType === 'threshold' ? thresholdDraft.name : eventDraft.name).trim()

    if (!name) {
      onError('Rule name is required.')
      return
    }

    let next: AlertRule
    if (ruleType === 'threshold') {
      const thresholdValue =
        thresholdDraft.metric === 'max_cost_per_hour'
          ? convertToUSD(Number(thresholdDraft.threshold), currency)
          : Number(thresholdDraft.threshold)
      if (thresholdDraft.destinationIds.length === 0) {
        onError('Select at least one destination.')
        return
      }
      if (!Number.isFinite(thresholdValue) || thresholdValue <= 0) {
        onError('Threshold must be greater than zero.')
        return
      }
      if (thresholdDraft.scope !== 'global' && !thresholdDraft.repository.trim()) {
        onError('Repository is required for this scope.')
        return
      }
      if (thresholdDraft.scope === 'job' && !thresholdDraft.jobId.trim()) {
        onError('Job ID is required for job scope.')
        return
      }
      next = {
        id: editingRuleId ?? crypto.randomUUID(),
        name,
        type: 'threshold',
        scope: thresholdDraft.scope,
        repository: thresholdDraft.repository.trim(),
        jobId: thresholdDraft.jobId.trim(),
        metric: thresholdDraft.metric,
        threshold: thresholdValue,
        destinationIds: thresholdDraft.destinationIds,
        enabled: true,
      }
    } else {
      const isGlobalEvent = eventDraft.event === 'daily_summary'
      if (eventDraft.destinationIds.length === 0) {
        onError('Select at least one destination.')
        return
      }
      if (eventDraft.event === 'policy_violation') {
        if (!eventDraft.policyKey) {
          onError('Select which policy this alert should track.')
          return
        }
        const picked = policyOptions.find((p) => p.key === eventDraft.policyKey)
        if (!picked) {
          onError('Selected policy could not be found.')
          return
        }
        next = {
          id: editingRuleId ?? crypto.randomUUID(),
          name,
          type: 'event',
          event: eventDraft.event,
          scope: picked.jobId ? 'job' : picked.repository ? 'repository' : 'global',
          repository: picked.repository,
          jobId: picked.jobId,
          metric: 'max_cost_per_hour',
          threshold: 0,
          destinationIds: eventDraft.destinationIds,
          enabled: true,
        }
      } else {
        if (!isGlobalEvent && eventDraft.scope !== 'global' && !eventDraft.repository.trim()) {
          onError('Repository is required for this scope.')
          return
        }
        if (!isGlobalEvent && eventDraft.scope === 'job' && !eventDraft.jobId.trim()) {
          onError('Job ID is required for job scope.')
          return
        }
        next = {
          id: editingRuleId ?? crypto.randomUUID(),
          name,
          type: 'event',
          event: eventDraft.event,
          scope: isGlobalEvent ? 'global' : eventDraft.scope,
          repository: isGlobalEvent ? '' : eventDraft.repository.trim(),
          jobId: isGlobalEvent ? '' : eventDraft.jobId.trim(),
          metric: 'max_cost_per_hour',
          threshold: 0,
          destinationIds: eventDraft.destinationIds,
          enabled: true,
        }
      }
    }

    setBusy(true)
    try {
      if (editingRuleId) {
        const nextRules = rules.map((r) => (r.id === editingRuleId ? { ...next, enabled: r.enabled } : r))
        await onRulesChange(nextRules)
        setEditingRuleId(null)
        onNote('Rule updated.')
      } else {
        const nextRules = [next, ...rules]
        await onRulesChange(nextRules)
        onNote('Alert rule added.')
      }

      // Reset forms
      const firstId = settings.slack.destinations?.[0]?.id
      setThresholdDraft({
        name: '',
        scope: 'global',
        repository: '',
        jobId: '',
        metric: 'max_cost_per_hour',
        threshold: displayCostFromUSD(0.5, currency),
        destinationIds: firstId ? [firstId] : [],
      })
      setEventDraft({
        name: '',
        event: 'policy_violation',
        policyKey: '',
        scope: 'global',
        repository: '',
        jobId: '',
        destinationIds: firstId ? [firstId] : [],
      })
      setPolicySearch('')
    } finally {
      setBusy(false)
    }
  }

  function startEditRule(rule: AlertRule) {
    setEditingRuleId(rule.id)
    setRuleType(rule.type)
    if (rule.type === 'threshold') {
      setThresholdDraft({
        name: rule.name,
        scope: rule.scope,
        repository: rule.repository,
        jobId: rule.jobId,
        metric: rule.metric,
        threshold:
          rule.metric === 'max_cost_per_hour' ? displayCostFromUSD(rule.threshold, currency) : String(rule.threshold),
        destinationIds: rule.destinationIds,
      })
    } else {
      setEventDraft({
        name: rule.name,
        event: rule.event ?? 'policy_violation',
        policyKey: `${rule.repository}::${rule.jobId}`,
        scope: rule.scope,
        repository: rule.repository,
        jobId: rule.jobId,
        destinationIds: rule.destinationIds,
      })
      const label =
        rule.repository && rule.jobId
          ? `${rule.repository} / ${rule.jobId}`
          : rule.repository
            ? `${rule.repository} (repo default)`
            : 'Global policy'
      setPolicySearch(label)
    }
    window.scrollTo({ top: 0, behavior: 'smooth' })
  }

  function cancelEdit() {
    setEditingRuleId(null)
    const firstId = settings.slack.destinations?.[0]?.id
    setThresholdDraft((prev) => ({
      ...prev,
      name: '',
      repository: '',
      jobId: '',
      scope: 'global',
      metric: 'max_cost_per_hour',
      threshold: displayCostFromUSD(0.5, currency),
      destinationIds: firstId ? [firstId] : [],
    }))
    setEventDraft((prev) => ({
      ...prev,
      name: '',
      event: 'policy_violation',
      policyKey: '',
      repository: '',
      jobId: '',
      scope: 'global',
      destinationIds: firstId ? [firstId] : [],
    }))
    setPolicySearch('')
    setRuleType('threshold')
  }

  async function removeRule(id: string) {
    setBusy(true)
    try {
      const nextRules = rules.filter((r) => r.id !== id)
      await onRulesChange(nextRules)
      onNote('Rule deleted.')
    } finally {
      setBusy(false)
    }
  }

  function confirmRemoveRule(rule: AlertRule) {
    setConfirmModal({ id: rule.id, name: rule.name })
  }

  async function toggleRuleEnabled(id: string, enabled: boolean) {
    setBusy(true)
    try {
      const nextRules = rules.map((r) => (r.id === id ? { ...r, enabled } : r))
      await onRulesChange(nextRules)
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
    <div className="rr-card">
      <h2 className="font-serif text-[17px] font-bold text-[var(--text)] mb-1">Alert Rules</h2>
      <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">
        <strong>Threshold</strong> — fires when a metric crosses a number you set.&nbsp;&nbsp;
        <strong>Event</strong> — fires when RunRight detects a policy breach, high waste, or sends a daily digest.
      </p>

      {(() => {
        const allDests = [
          ...settings.slack.destinations,
          ...settings.teams.destinations,
          ...(settings.webhooks?.destinations ?? []),
        ]
        if (allDests.length === 0) {
          return (
            <div className="flex items-start gap-3 rounded-lg border border-[var(--gold)]/40 bg-[var(--gold)]/5 px-4 py-3 mb-5">
              <svg className="mt-0.5 shrink-0 w-4 h-4 text-[var(--gold)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3m0 4h.01M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
              </svg>
              <p className="text-sm text-[var(--text-mid)]">
                <span className="font-semibold text-[var(--text)]">No destinations configured.</span>{' '}
                Alert rules require at least one notification destination (Slack, Teams, or Webhook) before they can fire.{' '}
                {onSwitchTab && (
                  <button
                    onClick={() => onSwitchTab('destinations')}
                    className="underline text-[var(--gold)] hover:text-[var(--gold-light)] transition-colors"
                  >
                    Add a destination
                  </button>
                )}
              </p>
            </div>
          )
        }
        return null
      })()}

      {/* ── Create / Edit rule form ── */}
      <div className={`rounded-lg border mb-6 overflow-visible ${editingRuleId ? 'border-[var(--gold)]' : 'border-[var(--border)]'}`}>
        <div className="flex items-center justify-between px-4 py-2.5 bg-[var(--cream-alt)] border-b border-[var(--border)] rounded-t-lg">
          <span className="font-serif font-semibold text-sm text-[var(--text)]">
            {editingRuleId ? 'Edit Rule' : 'New Alert Rule'}
          </span>
          {editingRuleId && (
            <button type="button" onClick={cancelEdit} className="text-xs text-[var(--text-light)] hover:text-[var(--text)] transition-colors">
              Cancel
            </button>
          )}
        </div>
        <form onSubmit={addRule} className="p-4 space-y-4 bg-[var(--paper)] rounded-b-lg">
          {/* Rule type toggle */}
          <div className="flex gap-2">
            {(['threshold', 'event'] as const).map((t) => (
              <button
                key={t}
                type="button"
                onClick={() => setRuleType(t)}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded text-xs font-semibold uppercase tracking-wide transition-colors border ${
                  ruleType === t
                    ? 'bg-[var(--gold)] text-[var(--text)] border-[var(--gold)]'
                    : 'bg-transparent text-[var(--text-light)] border-[var(--border)] hover:border-[var(--border-dark)] hover:text-[var(--text-mid)]'
                }`}
              >
                {t === 'event' ? icons.event : icons.threshold}
                {t}
              </button>
            ))}
          </div>

          {/* Name */}
          <div>
            <label className="block text-xs font-medium text-[var(--text-mid)] mb-1">Rule name</label>
            <input
              type="text"
              placeholder="e.g. High cost on prod jobs"
              value={ruleType === 'threshold' ? thresholdDraft.name : eventDraft.name}
              onChange={(e) =>
                ruleType === 'threshold'
                  ? setThresholdDraft((p) => ({ ...p, name: e.target.value }))
                  : setEventDraft((p) => ({ ...p, name: e.target.value }))
              }
              className="rr-input w-full"
            />
          </div>

          {/* ── Threshold-specific fields ── */}
          {ruleType === 'threshold' && (
            <>
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="block text-xs font-medium text-[var(--text-mid)] mb-1">Scope</label>
                  <select
                    className="rr-select w-full"
                    value={thresholdDraft.scope}
                    onChange={(e) =>
                      setThresholdDraft((p) => ({ ...p, scope: e.target.value as ThresholdRuleDraft['scope'], repository: '', jobId: '' }))
                    }
                  >
                    <option value="global">Global</option>
                    <option value="repository">Repository</option>
                    <option value="job">Job</option>
                  </select>
                </div>
                <div>
                  <label className="block text-xs font-medium text-[var(--text-mid)] mb-1">Metric</label>
                  <select
                    className="rr-select w-full"
                    value={thresholdDraft.metric}
                    onChange={(e) => {
                      const m = e.target.value as ThresholdRuleDraft['metric']
                      setThresholdDraft((p) => ({
                        ...p,
                        metric: m,
                        threshold: m === 'max_cost_per_hour' ? displayCostFromUSD(0.5, currency) : '80',
                      }))
                    }}
                  >
                    <option value="max_cost_per_hour">Cost per hour</option>
                    <option value="waste_percent">Waste %</option>
                    <option value="monthly_savings_drop_percent">Savings drop %</option>
                  </select>
                </div>
              </div>

              {(thresholdDraft.scope === 'repository' || thresholdDraft.scope === 'job') && (
                <div className="relative">
                  <label className="block text-xs font-medium text-[var(--text-mid)] mb-1">Repository</label>
                  <input
                    type="text"
                    placeholder="org/repo"
                    value={thresholdDraft.repository}
                    onFocus={() => setShowThresholdRepoSuggestions(true)}
                    onBlur={() => setTimeout(() => setShowThresholdRepoSuggestions(false), 150)}
                    onChange={(e) => setThresholdDraft((p) => ({ ...p, repository: e.target.value }))}
                    className="rr-input w-full"
                  />
                  {showThresholdRepoSuggestions && repoSuggestions(thresholdDraft.repository).length > 0 && (
                    <div className="absolute z-30 top-full left-0 right-0 mt-1 bg-[var(--paper)] border border-[var(--border)] rounded shadow-md overflow-auto max-h-40">
                      {repoSuggestions(thresholdDraft.repository).map((r) => (
                        <button key={r} type="button" onMouseDown={() => setThresholdDraft((p) => ({ ...p, repository: r }))}
                          className="w-full text-left px-3 py-1.5 text-sm text-[var(--text)] hover:bg-[var(--cream-alt)] transition-colors">{r}</button>
                      ))}
                    </div>
                  )}
                </div>
              )}

              {thresholdDraft.scope === 'job' && (
                <div className="relative">
                  <label className="block text-xs font-medium text-[var(--text-mid)] mb-1">Job ID</label>
                  <input
                    type="text"
                    placeholder="job-name"
                    value={thresholdDraft.jobId}
                    onFocus={() => setShowThresholdJobSuggestions(true)}
                    onBlur={() => setTimeout(() => setShowThresholdJobSuggestions(false), 150)}
                    onChange={(e) => setThresholdDraft((p) => ({ ...p, jobId: e.target.value }))}
                    className="rr-input w-full"
                  />
                  {showThresholdJobSuggestions && jobSuggestions(thresholdDraft.jobId).length > 0 && (
                    <div className="absolute z-30 top-full left-0 right-0 mt-1 bg-[var(--paper)] border border-[var(--border)] rounded shadow-md overflow-auto max-h-40">
                      {jobSuggestions(thresholdDraft.jobId).map((j) => (
                        <button key={j} type="button" onMouseDown={() => setThresholdDraft((p) => ({ ...p, jobId: j }))}
                          className="w-full text-left px-3 py-1.5 text-sm text-[var(--text)] hover:bg-[var(--cream-alt)] transition-colors">{j}</button>
                      ))}
                    </div>
                  )}
                </div>
              )}

              <div>
                <label className="block text-xs font-medium text-[var(--text-mid)] mb-1">
                  Threshold ({thresholdHint(thresholdDraft.metric)})
                </label>
                <div className="flex items-center gap-2">
                  <input
                    type="number"
                    step={thresholdStep(thresholdDraft.metric)}
                    min="0"
                    placeholder={thresholdPlaceholder(thresholdDraft.metric)}
                    value={thresholdDraft.threshold}
                    onChange={(e) => setThresholdDraft((p) => ({ ...p, threshold: e.target.value }))}
                    className="rr-input w-40"
                  />
                  <span className="text-sm text-[var(--text-light)]">
                    {thresholdDraft.metric === 'max_cost_per_hour' ? `${currency}/hr` : thresholdUnit(thresholdDraft.metric)}
                  </span>
                </div>
              </div>
            </>
          )}

          {/* ── Event-specific fields ── */}
          {ruleType === 'event' && (
            <>
              <div>
                <label className="block text-xs font-medium text-[var(--text-mid)] mb-1">Event type</label>
                <select
                  className="rr-select w-full"
                  value={eventDraft.event}
                  onChange={(e) => setEventDraft((p) => ({ ...p, event: e.target.value as EventType, policyKey: '' }))}
                >
                  <option value="policy_violation">Policy Violation</option>
                  <option value="high_waste">High Waste</option>
                  <option value="daily_summary">Daily Summary</option>
                </select>
              </div>

              {eventDraft.event === 'policy_violation' && (
                <div className="relative">
                  <label className="block text-xs font-medium text-[var(--text-mid)] mb-1">Policy to watch</label>
                  <input
                    type="text"
                    placeholder="Search policies…"
                    value={policySearch}
                    onFocus={() => setShowPolicySuggestions(true)}
                    onBlur={() => setTimeout(() => setShowPolicySuggestions(false), 150)}
                    onChange={(e) => { setPolicySearch(e.target.value); setEventDraft((p) => ({ ...p, policyKey: '' })) }}
                    className="rr-input w-full"
                  />
                  {showPolicySuggestions && filteredPolicyOptions(policySearch).length > 0 && (
                    <div className="absolute z-30 top-full left-0 right-0 mt-1 bg-[var(--paper)] border border-[var(--border)] rounded shadow-md overflow-auto max-h-44">
                      {filteredPolicyOptions(policySearch).map((p) => (
                        <button key={p.key} type="button"
                          onMouseDown={() => { setEventDraft((prev) => ({ ...prev, policyKey: p.key })); setPolicySearch(p.label) }}
                          className="w-full text-left px-3 py-1.5 text-sm text-[var(--text)] hover:bg-[var(--cream-alt)] transition-colors">{p.label}</button>
                      ))}
                    </div>
                  )}
                  {policyOptions.length === 0 && (
                    <p className="text-xs text-[var(--text-light)] mt-1">No cost policies found. Add one in the Policies page first.</p>
                  )}
                </div>
              )}

              {eventDraft.event !== 'policy_violation' && eventDraft.event !== 'daily_summary' && (
                <div>
                  <label className="block text-xs font-medium text-[var(--text-mid)] mb-1">Scope</label>
                  <select
                    className="rr-select w-full"
                    value={eventDraft.scope}
                    onChange={(e) => setEventDraft((p) => ({ ...p, scope: e.target.value as EventRuleDraft['scope'], repository: '', jobId: '' }))}
                  >
                    <option value="global">Global</option>
                    <option value="repository">Repository</option>
                    <option value="job">Job</option>
                  </select>
                </div>
              )}

              {eventDraft.event !== 'policy_violation' && eventDraft.event !== 'daily_summary' &&
                (eventDraft.scope === 'repository' || eventDraft.scope === 'job') && (
                  <div className="relative">
                    <label className="block text-xs font-medium text-[var(--text-mid)] mb-1">Repository</label>
                    <input
                      type="text"
                      placeholder="org/repo"
                      value={eventDraft.repository}
                      onFocus={() => setShowEventRepoSuggestions(true)}
                      onBlur={() => setTimeout(() => setShowEventRepoSuggestions(false), 150)}
                      onChange={(e) => setEventDraft((p) => ({ ...p, repository: e.target.value }))}
                      className="rr-input w-full"
                    />
                    {showEventRepoSuggestions && repoSuggestions(eventDraft.repository).length > 0 && (
                      <div className="absolute z-30 top-full left-0 right-0 mt-1 bg-[var(--paper)] border border-[var(--border)] rounded shadow-md overflow-auto max-h-40">
                        {repoSuggestions(eventDraft.repository).map((r) => (
                          <button key={r} type="button" onMouseDown={() => setEventDraft((p) => ({ ...p, repository: r }))}
                            className="w-full text-left px-3 py-1.5 text-sm text-[var(--text)] hover:bg-[var(--cream-alt)] transition-colors">{r}</button>
                        ))}
                      </div>
                    )}
                  </div>
              )}

              {eventDraft.event !== 'policy_violation' && eventDraft.scope === 'job' && (
                <div className="relative">
                  <label className="block text-xs font-medium text-[var(--text-mid)] mb-1">Job ID</label>
                  <input
                    type="text"
                    placeholder="job-name"
                    value={eventDraft.jobId}
                    onFocus={() => setShowEventJobSuggestions(true)}
                    onBlur={() => setTimeout(() => setShowEventJobSuggestions(false), 150)}
                    onChange={(e) => setEventDraft((p) => ({ ...p, jobId: e.target.value }))}
                    className="rr-input w-full"
                  />
                  {showEventJobSuggestions && jobSuggestions(eventDraft.jobId).length > 0 && (
                    <div className="absolute z-30 top-full left-0 right-0 mt-1 bg-[var(--paper)] border border-[var(--border)] rounded shadow-md overflow-auto max-h-40">
                      {jobSuggestions(eventDraft.jobId).map((j) => (
                        <button key={j} type="button" onMouseDown={() => setEventDraft((p) => ({ ...p, jobId: j }))}
                          className="w-full text-left px-3 py-1.5 text-sm text-[var(--text)] hover:bg-[var(--cream-alt)] transition-colors">{j}</button>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </>
          )}

          {/* ── Destinations ── */}
          <div className="relative">
            <label className="block text-xs font-medium text-[var(--text-mid)] mb-1">Destinations</label>
            <button
              type="button"
              onClick={() => setShowDestinationPicker((p) => !p)}
              className="rr-input w-full text-left flex items-center justify-between gap-2"
            >
              <span className="text-sm truncate text-[var(--text)]">
                {activeDestinationIds.length === 0
                  ? 'Select destinations…'
                  : selectedDestinations.map((d) => d.name).join(', ')}
              </span>
              <svg className="shrink-0 w-4 h-4 text-[var(--text-light)]" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={1.5}>
                <path strokeLinecap="round" strokeLinejoin="round" d={showDestinationPicker ? 'M5 15l7-7 7 7' : 'M19 9l-7 7-7-7'} />
              </svg>
            </button>
            {showDestinationPicker && (
              <div className="absolute z-30 top-full left-0 right-0 mt-1 bg-[var(--paper)] border border-[var(--border)] rounded shadow-lg overflow-hidden">
                <div className="p-2 border-b border-[var(--border)]">
                  <input
                    type="text"
                    placeholder="Search destinations…"
                    value={destinationSearch}
                    onChange={(e) => setDestinationSearch(e.target.value)}
                    className="rr-input w-full text-sm"
                  />
                </div>
                <div className="overflow-auto max-h-44">
                  {allDestinations
                    .filter((d) => {
                      const q = destinationSearch.trim().toLowerCase()
                      return !q || d.name.toLowerCase().includes(q) || d.type.toLowerCase().includes(q)
                    })
                    .map((dest) => (
                      <label key={dest.id} className="flex items-center gap-3 px-3 py-2 hover:bg-[var(--cream-alt)] cursor-pointer">
                        <input
                          type="checkbox"
                          checked={activeDestinationIds.includes(dest.id)}
                          onChange={(e) => toggleRuleDestination(dest.id, e.target.checked)}
                          className="accent-[var(--gold)] w-4 h-4 shrink-0"
                        />
                        <span className="flex-1 text-sm text-[var(--text)] truncate">{dest.name}</span>
                        <span className="text-[10px] font-semibold px-1.5 py-0.5 rounded bg-[var(--cream-alt)] text-[var(--text-light)] uppercase tracking-wide border border-[var(--border)]">{dest.type}</span>
                      </label>
                    ))}
                  {allDestinations.length === 0 && (
                    <p className="px-3 py-4 text-sm text-[var(--text-light)] text-center">No destinations configured yet.</p>
                  )}
                </div>
              </div>
            )}
          </div>

          {/* ── Actions ── */}
          <div className="flex items-center gap-2 pt-1">
            <button type="submit" disabled={busy || !can('alerts:manage')} title={!can('alerts:manage') ? 'Requires admin or owner role' : undefined} className="btn-rr">
              {busy ? 'Saving…' : editingRuleId ? 'Update Rule' : 'Add Rule'}
            </button>
            {editingRuleId && (
              <button
                type="button"
                onClick={cancelEdit}
                disabled={busy}
                className="px-4 py-2 text-sm border border-[var(--border)] rounded text-[var(--text-mid)] hover:border-[var(--border-dark)] hover:text-[var(--text)] transition-colors"
              >
                Cancel
              </button>
            )}
          </div>
        </form>
      </div>

      <div className="mt-6">
        {rules.length === 0 ? (
          <div className="empty text-base">No alert rules yet.</div>
        ) : (
          <>
            {/* Search bar */}
            <div className="mb-4">
              <input
                type="text"
                placeholder="Search rules by name, repository, or destination..."
                value={rulesSearchQuery}
                onChange={(e) => setRulesSearchQuery(e.target.value)}
                className="w-full px-3 py-2 border border-[var(--border)] rounded bg-[var(--cream)] text-sm placeholder:text-[var(--text-light)] focus:outline-none focus:border-[var(--gold)]"
              />
            </div>
            <div className="space-y-3">
              {rules
                .filter((rule) => {
                  if (!rulesSearchQuery.trim()) return true
                  const q = rulesSearchQuery.toLowerCase()
                  return (
                    rule.name.toLowerCase().includes(q) ||
                    rule.repository?.toLowerCase().includes(q) ||
                    rule.jobId?.toLowerCase().includes(q) ||
                    rule.type.toLowerCase().includes(q) ||
                    destinationNames(rule.destinationIds).toLowerCase().includes(q)
                  )
                })
                .map((rule) => (
                  <div
                    key={rule.id}
                    className={`bg-paper border rounded px-4 py-3 ${
                      editingRuleId === rule.id ? 'border-[var(--gold)]' : 'border-[var(--border)]'
                    }`}
                  >
                    <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-3">
                      <div>
                        <div className="flex items-center gap-2">
                          <div className="font-sans font-semibold text-sm text-[var(--text)]">{rule.name}</div>
                          <span
                            className={`inline-flex items-center gap-1 text-[10px] font-bold px-1.5 py-0.5 rounded uppercase tracking-wide ${
                              rule.type === 'event'
                                ? 'bg-[var(--navy)] text-[var(--cream)]'
                                : 'bg-[var(--gold)] text-[var(--text)]'
                            }`}
                          >
                            {rule.type === 'event' ? icons.event : icons.threshold}
                            {rule.type}
                          </span>
                        </div>
                        <div className="text-xs text-[var(--text-light)] mt-1">
                          {rule.type === 'event'
                            ? `${eventLabel(rule.event ?? 'policy_violation')}${rule.scope !== 'global' ? ` · ${rule.scope === 'job' ? `${rule.repository} / ${rule.jobId}` : rule.repository}` : ''}`
                            : `${rule.scope.toUpperCase()} · ${metricLabel(rule.metric)} > ${rule.metric === 'max_cost_per_hour' ? `${displayCostFromUSD(rule.threshold, currency)}/hr` : `${rule.threshold} ${thresholdUnit(rule.metric)}`}${rule.repository ? ` · ${rule.repository}` : ''}${rule.jobId ? ` / ${rule.jobId}` : ''}`}
                        </div>
                        <div className="text-xs text-[var(--text-light)] mt-1">→ {destinationNames(rule.destinationIds)}</div>
                      </div>
                      <div className="flex items-center gap-3 shrink-0">
                        <label className="rr-switch-row rr-switch-label">
                          <input
                            className="rr-switch"
                            type="checkbox"
                            checked={rule.enabled}
                            onChange={(e) => void toggleRuleEnabled(rule.id, e.target.checked)}
                            disabled={busy}
                            title={rule.enabled ? 'Disable alert rule' : 'Enable alert rule'}
                            aria-label={rule.enabled ? 'Disable alert rule' : 'Enable alert rule'}
                          />
                          <span className={`rr-switch-state ${rule.enabled ? 'on' : 'off'}`}>
                            {rule.enabled ? 'Enabled' : 'Disabled'}
                          </span>
                        </label>
                        <button
                          type="button"
                          className="inline-flex h-7 w-7 items-center justify-center rounded border border-[var(--border)] text-[var(--text-light)] hover:border-[var(--border-dark)] hover:text-[var(--text-mid)] hover:bg-[var(--cream-alt)]"
                          onClick={() => startEditRule(rule)}
                          disabled={busy}
                          title="Edit rule"
                          aria-label="Edit rule"
                        >
                          {icons.edit}
                        </button>
                        <button
                          type="button"
                          className="inline-flex h-7 w-7 items-center justify-center rounded border border-[var(--border)] text-[var(--text-light)] hover:border-[var(--red-dark)] hover:text-[var(--red-dark)] hover:bg-[rgba(194,59,34,.08)]"
                          onClick={() => confirmRemoveRule(rule)}
                          disabled={busy}
                          title="Delete rule"
                          aria-label="Delete rule"
                        >
                          {icons.trash}
                        </button>
                      </div>
                    </div>
                  </div>
                ))}
              {rules.filter((rule) => {
                if (!rulesSearchQuery.trim()) return true
                const q = rulesSearchQuery.toLowerCase()
                return (
                  rule.name.toLowerCase().includes(q) ||
                  rule.repository?.toLowerCase().includes(q) ||
                  rule.jobId?.toLowerCase().includes(q) ||
                  rule.type.toLowerCase().includes(q) ||
                  destinationNames(rule.destinationIds).toLowerCase().includes(q)
                )
              }).length === 0 && rulesSearchQuery.trim() && (
                <div className="text-sm text-[var(--text-light)] text-center py-6">
                  No rules match your search.
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </div>
    {confirmModal && (
      <ConfirmModal
        open={true}
        title="Delete alert rule"
        message={`Permanently delete the rule "${confirmModal.name}"? Any associated delivery history will remain but no new alerts will fire.`}
        confirmLabel="Delete Rule"
        danger
        onConfirm={() => { setConfirmModal(null); void removeRule(confirmModal.id) }}
        onCancel={() => setConfirmModal(null)}
      />
    )}
    </>
  )
}
