import { useEffect, useState } from 'react'
import { fetchNotificationSettings, upsertNotificationSettings } from '../api'
import DestinationsTab from './AlertsPage/DestinationsTab'
import DeliveryLogsTab from './AlertsPage/DeliveryLogsTab'
import OwnershipTab from './AlertsPage/OwnershipTab'
import RulesTab from './AlertsPage/RulesTab'
import type { NotificationSettings } from '../types'
import type { AlertRule } from './AlertsPage/types'

type TabId = 'rules' | 'destinations' | 'ownership' | 'deliveries'

const DEFAULT_SETTINGS: NotificationSettings = {
  enabled: true,
  events: {
    policy_violation: true,
    high_waste: false,
    daily_summary: true,
  },
  slack: {
    enabled: true,
    webhook_url: '',
    channel: '',
    mention: '',
    destinations: [],
  },
  teams: {
    enabled: false,
    destinations: [],
  },
  webhooks: {
    enabled: false,
    destinations: [],
  },
  email: {
    enabled: false,
    recipients: [],
    subject_prefix: '[RunRight]',
  },
  rules: [],
}

function normalizeSettings(input?: Partial<NotificationSettings> | null): NotificationSettings {
  if (!input) return DEFAULT_SETTINGS
  return {
    enabled: input.enabled ?? DEFAULT_SETTINGS.enabled,
    events: {
      policy_violation: input.events?.policy_violation ?? DEFAULT_SETTINGS.events.policy_violation,
      high_waste: input.events?.high_waste ?? DEFAULT_SETTINGS.events.high_waste,
      daily_summary: input.events?.daily_summary ?? DEFAULT_SETTINGS.events.daily_summary,
    },
    slack: {
      enabled: input.slack?.enabled ?? DEFAULT_SETTINGS.slack.enabled,
      webhook_url: input.slack?.webhook_url ?? DEFAULT_SETTINGS.slack.webhook_url,
      channel: input.slack?.channel ?? DEFAULT_SETTINGS.slack.channel,
      mention: input.slack?.mention ?? DEFAULT_SETTINGS.slack.mention,
      destinations: Array.isArray(input.slack?.destinations) ? input.slack!.destinations : [],
    },
    teams: {
      enabled: input.teams?.enabled ?? false,
      destinations: Array.isArray(input.teams?.destinations) ? input.teams!.destinations : [],
    },
    webhooks: {
      enabled: input.webhooks?.enabled ?? false,
      destinations: Array.isArray(input.webhooks?.destinations) ? input.webhooks!.destinations : [],
    },
    email: {
      enabled: input.email?.enabled ?? DEFAULT_SETTINGS.email.enabled,
      recipients: input.email?.recipients ?? DEFAULT_SETTINGS.email.recipients,
      subject_prefix: input.email?.subject_prefix ?? DEFAULT_SETTINGS.email.subject_prefix,
    },
    rules: Array.isArray(input.rules) ? (input.rules as AlertRule[]) : [],
  }
}

export default function AlertsPage() {
  const [activeTab, setActiveTab] = useState<TabId>('rules')
  const [settings, setSettings] = useState<NotificationSettings>(DEFAULT_SETTINGS)
  const [rules, setRules] = useState<AlertRule[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [note, setNote] = useState('')

  // Load settings on mount
  useEffect(() => {
    void loadSettings()
  }, [])

  const loadSettings = async () => {
    try {
      setLoading(true)
      const fetched = await fetchNotificationSettings()
      const normalized = normalizeSettings(fetched)
      setSettings(normalized)
      setRules(normalized.rules || [])
    } catch {
      setError('Failed to load notification settings.')
    } finally {
      setLoading(false)
    }
  }

  const handleRulesChange = async (nextRules: AlertRule[]) => {
    setRules(nextRules)
    const updated: NotificationSettings = {
      ...settings,
      rules: nextRules,
    }
    try {
      await upsertNotificationSettings(updated)
      setSettings(updated)
    } catch (err) {
      setError('Failed to save rules.')
      throw err
    }
  }

  const handleSettingsChange = async (updated: NotificationSettings) => {
    setSettings(updated)
    try {
      await upsertNotificationSettings(updated)
    } catch (err) {
      setError('Failed to save settings.')
      throw err
    }
  }

  const tabBtn = (tab: TabId, label: string) => (
    <button
      type="button"
      onClick={() => setActiveTab(tab)}
      className={`px-5 py-2.5 font-deco text-[13px] tracking-[1.5px] uppercase border-b-2 transition-colors ${
        activeTab === tab
          ? 'border-[var(--red)] text-[var(--text)]'
          : 'border-transparent text-[var(--text-light)] hover:text-[var(--text-mid)]'
      }`}
    >
      {label}
    </button>
  )

  if (loading) {
    return (
      <div className="max-w-[1280px] fadein">
        <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] mb-5 tracking-tight">Alerts</h1>
        <p className="text-sm text-[var(--text-light)]">Loading notification settings…</p>
      </div>
    )
  }

  return (
    <div className="fadein max-w-[1280px]">
      <h1 className="font-serif text-2xl sm:text-3xl font-black text-[var(--text)] mb-5 tracking-tight">Alerts</h1>

      {/* Tab bar */}
      <div className="flex gap-0 border-b border-[var(--border)] mb-6 flex-wrap">
        {tabBtn('rules', 'Rules')}
        {tabBtn('destinations', 'Destinations')}
        {tabBtn('ownership', 'Ownership')}
        {tabBtn('deliveries', 'Delivery Logs')}
      </div>

      {/* Error/Note messages */}
      {error && (
        <div className="bg-[rgba(194,59,34,.10)] border border-[var(--red)] rounded px-4 py-3 mb-6 text-sm text-[var(--red)]">
          {error}
          <button
            type="button"
            className="float-right underline underline-offset-2 text-xs"
            onClick={() => setError('')}
          >
            Dismiss
          </button>
        </div>
      )}
      {note && (
        <div className="bg-[rgba(46,125,50,.10)] border border-[#2E7D32] rounded px-4 py-3 mb-6 text-sm text-[#2E7D32]">
          {note}
          <button
            type="button"
            className="float-right underline underline-offset-2 text-xs"
            onClick={() => setNote('')}
          >
            Dismiss
          </button>
        </div>
      )}

      {/* Rules Tab */}
      {activeTab === 'rules' && (
        <RulesTab
          rules={rules}
          settings={settings}
          onRulesChange={handleRulesChange}
          onError={(msg) => setError(msg)}
          onNote={(msg) => setNote(msg)}
          onSwitchTab={setActiveTab}
        />
      )}

      {/* Destinations Tab */}
      {activeTab === 'destinations' && (
        <DestinationsTab
          settings={settings}
          onSettingsChange={handleSettingsChange}
          onError={(msg) => setError(msg)}
          onNote={(msg) => setNote(msg)}
        />
      )}

      {/* Ownership Tab */}
      {activeTab === 'ownership' && (
        <OwnershipTab
          settings={settings}
          onError={(msg) => setError(msg)}
          onNote={(msg) => setNote(msg)}
        />
      )}

      {/* Delivery Logs Tab */}
      {activeTab === 'deliveries' && <DeliveryLogsTab onError={(msg) => setError(msg)} />}
    </div>
  )
}
