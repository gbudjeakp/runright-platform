import { useState } from 'react'
import type { NotificationSettings } from '../../types'
import type { SlackDestination, TeamsDestination, WebhookDestination } from './types'
import { useUser } from '../../App'
import { ConfirmModal } from '../../components/ConfirmModal'

const isProbablyWebhook = (url: string): boolean => {
  try {
    const parsed = new URL(url)
    return parsed.protocol === 'https:' || parsed.protocol === 'http:'
  } catch {
    return false
  }
}

export interface DestinationsTabProps {
  settings: NotificationSettings
  onSettingsChange: (settings: NotificationSettings) => Promise<void>
  onError: (msg: string) => void
  onNote: (msg: string) => void
}

type DestinationType = 'slack' | 'teams' | 'webhooks' | 'email'

export default function DestinationsTab({ settings, onSettingsChange, onError, onNote }: DestinationsTabProps) {
  const { can } = useUser()
  const canWrite = can('alerts:manage')
  const [destinationType, setDestinationType] = useState<DestinationType>('slack')
  const [busy, setBusy] = useState(false)
  const [confirmRemove, setConfirmRemove] = useState<{ type: DestinationType; id: string; name: string } | null>(null)
  const [newDestinationName, setNewDestinationName] = useState('')
  const [newDestinationWebhook, setNewDestinationWebhook] = useState('')
  const [newDestinationChannel, setNewDestinationChannel] = useState('')
  const [newDestinationMention, setNewDestinationMention] = useState('')
  const [newEmailRecipient, setNewEmailRecipient] = useState('')
  const [emailSubjectPrefix, setEmailSubjectPrefix] = useState(settings.email?.subject_prefix || '[RunRight]')

  async function persistSettings(updated: NotificationSettings) {
    const payload = {
      ...updated,
      enabled: updated.slack.destinations.length > 0 ||
        (updated.teams?.destinations?.length ?? 0) > 0 ||
        (updated.webhooks?.destinations?.length ?? 0) > 0 ||
        (updated.email?.recipients?.length ?? 0) > 0,
    }
    payload.slack.enabled = payload.slack.destinations.length > 0
    if (payload.teams) payload.teams.enabled = (payload.teams.destinations?.length ?? 0) > 0
    if (payload.webhooks) payload.webhooks.enabled = (payload.webhooks?.destinations?.length ?? 0) > 0
    if (payload.email) payload.email.enabled = (payload.email.recipients?.length ?? 0) > 0

    const invalid = payload.slack.destinations.find(
      (d) => !d.has_secret && !isProbablyWebhook(d.webhook_url || '')
    )
    if (invalid) {
      onError(`Invalid webhook URL for destination: ${invalid.name}.`)
      return
    }

    if (payload.slack.destinations.length > 0) {
      payload.slack.webhook_url = payload.slack.destinations[0].webhook_url
      payload.slack.channel = payload.slack.destinations[0].channel
      payload.slack.mention = payload.slack.destinations[0].mention
    }

    try {
      await onSettingsChange(payload)
      onNote('Saved.')
    } catch {
      onError('Unable to save. Check backend and try again.')
    }
  }

  async function addSlackDestination(e: React.FormEvent) {
    e.preventDefault()
    onError('')
    const name = newDestinationName.trim()
    const webhook = newDestinationWebhook.trim()

    if (!name) {
      onError('Name is required.')
      return
    }
    if (!isProbablyWebhook(webhook)) {
      onError('A valid Slack webhook URL is required.')
      return
    }

    const next: SlackDestination = {
      id: crypto.randomUUID(),
      name,
      webhook_url: webhook,
      channel: newDestinationChannel.trim(),
      mention: newDestinationMention.trim(),
    }

    const updated: NotificationSettings = {
      ...settings,
      slack: {
        ...settings.slack,
        destinations: [...settings.slack.destinations, next],
      },
    }

    setNewDestinationName('')
    setNewDestinationWebhook('')
    setNewDestinationChannel('')
    setNewDestinationMention('')
    setBusy(true)
    await persistSettings(updated)
    setBusy(false)
  }

  async function addTeamsDestination(e: React.FormEvent) {
    e.preventDefault()
    onError('')
    const name = newDestinationName.trim()
    const webhook = newDestinationWebhook.trim()

    if (!name) {
      onError('Name is required.')
      return
    }
    if (!isProbablyWebhook(webhook)) {
      onError('A valid Teams webhook URL is required.')
      return
    }

    const next: TeamsDestination = {
      id: crypto.randomUUID(),
      name,
      webhook_url: webhook,
    }

    const updated: NotificationSettings = {
      ...settings,
      teams: {
        enabled: true,
        destinations: [...(settings.teams?.destinations ?? []), next],
      },
    }

    setNewDestinationName('')
    setNewDestinationWebhook('')
    setBusy(true)
    await persistSettings(updated)
    setBusy(false)
  }

  async function addWebhookDestination(e: React.FormEvent) {
    e.preventDefault()
    onError('')
    const name = newDestinationName.trim()
    const url = newDestinationWebhook.trim()

    if (!name) {
      onError('Name is required.')
      return
    }
    if (!isProbablyWebhook(url)) {
      onError('A valid webhook URL is required.')
      return
    }

    const next: WebhookDestination = {
      id: crypto.randomUUID(),
      name,
      url,
    }

    const updated: NotificationSettings = {
      ...settings,
      webhooks: {
        enabled: true,
        destinations: [...(settings.webhooks?.destinations ?? []), next],
      },
    }

    setNewDestinationName('')
    setNewDestinationWebhook('')
    setBusy(true)
    await persistSettings(updated)
    setBusy(false)
  }

  async function removeDestination(type: DestinationType, id: string) {
    setBusy(true)
    const updated: NotificationSettings = {
      ...settings,
      [type === 'slack'
        ? 'slack'
        : type === 'teams'
          ? 'teams'
          : 'webhooks']:
        type === 'slack'
          ? { ...settings.slack, destinations: settings.slack.destinations.filter((x) => x.id !== id) }
          : type === 'teams'
            ? { ...settings.teams!, destinations: (settings.teams?.destinations ?? []).filter((x) => x.id !== id) }
            : { ...settings.webhooks!, destinations: (settings.webhooks?.destinations ?? []).filter((x) => x.id !== id) },
    }
    await persistSettings(updated)
    setBusy(false)
  }

  function confirmRemoveDestination(type: DestinationType, id: string, name: string) {
    setConfirmRemove({ type, id, name })
  }

  async function addEmailRecipient(e: React.FormEvent) {
    e.preventDefault()
    onError('')
    const email = newEmailRecipient.trim()

    if (!email) {
      onError('Email address is required.')
      return
    }
    // Simple email validation
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
      onError('Please enter a valid email address.')
      return
    }
    if (settings.email?.recipients?.includes(email)) {
      onError('This email address is already added.')
      return
    }

    const updated: NotificationSettings = {
      ...settings,
      email: {
        enabled: true,
        recipients: [...(settings.email?.recipients ?? []), email],
        subject_prefix: emailSubjectPrefix,
      },
    }

    setNewEmailRecipient('')
    setBusy(true)
    await persistSettings(updated)
    setBusy(false)
  }

  async function removeEmailRecipient(email: string) {
    setBusy(true)
    const updated: NotificationSettings = {
      ...settings,
      email: {
        ...settings.email,
        enabled: (settings.email?.recipients ?? []).filter((e) => e !== email).length > 0,
        recipients: (settings.email?.recipients ?? []).filter((e) => e !== email),
        subject_prefix: emailSubjectPrefix,
      },
    }
    await persistSettings(updated)
    setBusy(false)
  }

  async function updateEmailSubjectPrefix() {
    setBusy(true)
    const updated: NotificationSettings = {
      ...settings,
      email: {
        ...settings.email,
        enabled: settings.email?.enabled ?? false,
        recipients: settings.email?.recipients ?? [],
        subject_prefix: emailSubjectPrefix,
      },
    }
    await persistSettings(updated)
    setBusy(false)
  }

  return (
    <>
    <div className="rr-card max-w-3xl">
      <h2 className="font-serif text-[18px] font-bold text-[var(--text)] mb-1">Notification Destinations</h2>
      <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">
        Configure notification channels for alert rules: Slack, Microsoft Teams, email, or custom webhooks.
      </p>

      {/* Destination type sub-tabs */}
      <div className="flex gap-1 mb-6 border-b border-[var(--border)] pb-0">
        {(['slack', 'teams', 'webhooks', 'email'] as const).map((type) => (
          <button
            key={type}
            type="button"
            onClick={() => setDestinationType(type)}
            className={`px-4 py-2.5 font-deco text-[13px] tracking-[1.5px] uppercase border-b-2 transition-colors ${
              destinationType === type
                ? 'border-[var(--red)] text-[var(--text)]'
                : 'border-transparent text-[var(--text-light)] hover:text-[var(--text-mid)]'
            }`}
          >
            {type === 'slack' ? 'Slack' : type === 'teams' ? 'Teams' : type === 'webhooks' ? 'Webhooks' : 'Email'}
          </button>
        ))}
      </div>

      {/* Slack Destinations */}
      {destinationType === 'slack' && (
        <div>
          <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">
            Each destination is a Slack webhook URL. Add one destination per channel or team you want to notify.
          </p>
          {(settings.slack.destinations ?? []).length > 0 && (
            <div className="divide-y divide-[var(--border)] border border-[var(--border)] rounded mb-6">
              {settings.slack.destinations.map((d) => (
                <div key={d.id} className="flex items-center justify-between px-4 py-3 gap-3">
                  <div className="min-w-0">
                    <div className="font-semibold text-sm text-[var(--text)] truncate">{d.name}</div>
                    <div className="text-xs text-[var(--text-light)] mt-0.5">
                      {d.channel || '(default channel)'}
                      {d.mention ? ` · ${d.mention}` : ''}
                    </div>
                  </div>
                  <button
                    type="button"
                    className="flex-shrink-0 text-xs font-deco tracking-widest text-[var(--red)] border border-[var(--red)] px-3 py-1 rounded hover:bg-[rgba(194,59,34,.06)]"
                    onClick={() => confirmRemoveDestination('slack', d.id, d.name)}
                    disabled={busy || !canWrite}
                  >
                    Remove
                  </button>
                </div>
              ))}
            </div>
          )}
          <form onSubmit={(e) => void addSlackDestination(e)}>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-3 mb-3">
              <div className="form-group mb-0">
                <label>Name</label>
                <input
                  type="text"
                  placeholder="CI Alerts"
                  value={newDestinationName}
                  onChange={(e) => setNewDestinationName(e.target.value)}
                  disabled={busy || !canWrite}
                />
              </div>
              <div className="form-group mb-0">
                <label>Webhook URL</label>
                <input
                  type="text"
                  placeholder="https://hooks.slack.com/services/..."
                  value={newDestinationWebhook}
                  onChange={(e) => setNewDestinationWebhook(e.target.value)}
                  disabled={busy || !canWrite}
                />
              </div>
              <div className="form-group mb-0">
                <label>
                  Channel <span className="text-[var(--text-light)] font-normal">(optional)</span>
                </label>
                <input
                  type="text"
                  placeholder="#ci-alerts"
                  value={newDestinationChannel}
                  onChange={(e) => setNewDestinationChannel(e.target.value)}
                  disabled={busy || !canWrite}
                />
              </div>
              <div className="form-group mb-0">
                <label>
                  Mention <span className="text-[var(--text-light)] font-normal">(optional)</span>
                </label>
                <input
                  type="text"
                  placeholder="@oncall-team"
                  value={newDestinationMention}
                  onChange={(e) => setNewDestinationMention(e.target.value)}
                  disabled={busy || !canWrite}
                />
              </div>
            </div>
            <button className="btn-rr mt-3" type="submit" disabled={busy || !canWrite}>
              {busy ? 'Saving...' : 'Add Destination'}
            </button>
          </form>
        </div>
      )}

      {/* Teams Destinations */}
      {destinationType === 'teams' && (
        <div>
          <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">
            Add Teams Incoming Webhook URLs. Rules can route to any destination across Slack, Teams, and webhooks.
          </p>
          {(settings.teams?.destinations ?? []).length > 0 && (
            <div className="divide-y divide-[var(--border)] border border-[var(--border)] rounded mb-6">
              {settings.teams!.destinations.map((d) => (
                <div key={d.id} className="flex items-center justify-between px-4 py-3 gap-3">
                  <div className="min-w-0">
                    <div className="font-semibold text-sm text-[var(--text)] truncate">{d.name}</div>
                    <div className="text-xs text-[var(--text-light)] mt-0.5">
                      {d.has_secret ? 'Webhook saved ✓' : d.webhook_url || '—'}
                    </div>
                  </div>
                  <button
                    type="button"
                    className="flex-shrink-0 text-xs font-deco tracking-widest text-[var(--red)] border border-[var(--red)] px-3 py-1 rounded hover:bg-[rgba(194,59,34,.06)]"
                    onClick={() => confirmRemoveDestination('teams', d.id, d.name)}
                    disabled={busy || !canWrite}
                  >
                    Remove
                  </button>
                </div>
              ))}
            </div>
          )}
          <form className="settings-form" onSubmit={(e) => void addTeamsDestination(e)}>
            <div className="form-group">
              <label>Destination Name</label>
              <input
                type="text"
                placeholder="e.g. CI Alerts"
                value={newDestinationName}
                onChange={(e) => setNewDestinationName(e.target.value)}
                disabled={busy || !canWrite}
              />
            </div>
            <div className="form-group">
              <label>Teams Incoming Webhook URL</label>
              <input
                type="url"
                placeholder="https://outlook.office.com/webhook/..."
                value={newDestinationWebhook}
                onChange={(e) => setNewDestinationWebhook(e.target.value)}
                disabled={busy || !canWrite}
              />
            </div>
            <button type="submit" className="btn-rr" disabled={busy || !canWrite}>
              Add Teams Destination
            </button>
          </form>
        </div>
      )}

      {/* Webhooks Destinations */}
      {destinationType === 'webhooks' && (
        <div>
          <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">
            Send structured JSON payloads to any HTTP endpoint — PagerDuty, custom pipelines, Zapier, or your own webhook
            receiver.
          </p>
          {(settings.webhooks?.destinations ?? []).length > 0 && (
            <div className="divide-y divide-[var(--border)] border border-[var(--border)] rounded mb-6">
              {settings.webhooks!.destinations.map((d) => (
                <div key={d.id} className="flex items-center justify-between px-4 py-3 gap-3">
                  <div className="min-w-0">
                    <div className="font-semibold text-sm text-[var(--text)] truncate">{d.name}</div>
                    <div className="text-xs text-[var(--text-light)] mt-0.5">
                      {d.has_secret ? 'URL saved ✓' : d.url || '—'}
                    </div>
                  </div>
                  <button
                    type="button"
                    className="flex-shrink-0 text-xs font-deco tracking-widest text-[var(--red)] border border-[var(--red)] px-3 py-1 rounded hover:bg-[rgba(194,59,34,.06)]"
                    onClick={() => confirmRemoveDestination('webhooks', d.id, d.name)}
                    disabled={busy || !canWrite}
                  >
                    Remove
                  </button>
                </div>
              ))}
            </div>
          )}
          <form className="settings-form" onSubmit={(e) => void addWebhookDestination(e)}>
            <div className="form-group">
              <label>Destination Name</label>
              <input
                type="text"
                placeholder="e.g. PagerDuty"
                value={newDestinationName}
                onChange={(e) => setNewDestinationName(e.target.value)}
                disabled={busy || !canWrite}
              />
            </div>
            <div className="form-group">
              <label>Webhook URL</label>
              <input
                type="url"
                placeholder="https://..."
                value={newDestinationWebhook}
                onChange={(e) => setNewDestinationWebhook(e.target.value)}
                disabled={busy || !canWrite}
              />
            </div>
            <button type="submit" className="btn-rr" disabled={busy || !canWrite}>
              Add Webhook Destination
            </button>
          </form>
        </div>
      )}

      {/* Email Destinations */}
      {destinationType === 'email' && (
        <div>
          <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">
            Add email addresses to receive alert notifications. Each recipient will receive all alerts routed to the email channel.
          </p>

          {/* Subject Prefix */}
          <div className="mb-6">
            <label className="block text-sm font-medium text-[var(--text-mid)] mb-1">Subject Prefix</label>
            <div className="flex gap-2">
              <input
                type="text"
                placeholder="[RunRight]"
                value={emailSubjectPrefix}
                onChange={(e) => setEmailSubjectPrefix(e.target.value)}
                className="flex-1 rr-input text-sm"
                disabled={busy || !canWrite}
              />
              <button
                type="button"
                className="btn-rr text-sm"
                onClick={() => void updateEmailSubjectPrefix()}
                disabled={busy || emailSubjectPrefix === (settings.email?.subject_prefix || '[RunRight]')}
              >
                Save
              </button>
            </div>
            <p className="text-xs text-[var(--text-light)] mt-1">
              This prefix will appear at the beginning of all email subject lines.
            </p>
          </div>

          {/* Recipients List */}
          {(settings.email?.recipients ?? []).length > 0 && (
            <div className="divide-y divide-[var(--border)] border border-[var(--border)] rounded mb-6">
              {(settings.email?.recipients ?? []).map((email) => (
                <div key={email} className="flex items-center justify-between px-4 py-3 gap-3">
                  <div className="min-w-0">
                    <div className="font-semibold text-sm text-[var(--text)] truncate">{email}</div>
                  </div>
                  <button
                    type="button"
                    className="flex-shrink-0 text-xs font-deco tracking-widest text-[var(--red)] border border-[var(--red)] px-3 py-1 rounded hover:bg-[rgba(194,59,34,.06)]"
                    onClick={() => void removeEmailRecipient(email)}
                    disabled={busy || !canWrite}
                  >
                    Remove
                  </button>
                </div>
              ))}
            </div>
          )}

          {/* Add Recipient Form */}
          <form className="settings-form" onSubmit={(e) => void addEmailRecipient(e)}>
            <div className="form-group">
              <label>Email Address</label>
              <input
                type="email"
                placeholder="team@example.com"
                value={newEmailRecipient}
                onChange={(e) => setNewEmailRecipient(e.target.value)}
                disabled={busy || !canWrite}
              />
            </div>
            <button type="submit" className="btn-rr" disabled={busy || !canWrite}>
              Add Email Recipient
            </button>
          </form>
        </div>
      )}
    </div>
    {confirmRemove && (
      <ConfirmModal
        open={true}
        title="Remove destination"
        message={`Remove the "${confirmRemove.name}" destination? Existing alert rules that reference it will stop delivering to this channel.`}
        confirmLabel="Remove"
        danger
        onConfirm={() => { const { type, id } = confirmRemove; setConfirmRemove(null); void removeDestination(type, id) }}
        onCancel={() => setConfirmRemove(null)}
      />
    )}
    </>
  )
}
