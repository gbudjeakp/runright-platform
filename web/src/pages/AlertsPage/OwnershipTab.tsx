import { useEffect, useState } from 'react'
import { deleteOwnership, fetchOwnership, upsertOwnership } from '../../api'
import type { NotificationSettings, OwnershipEntry } from '../../types'

export interface OwnershipTabProps {
  settings: NotificationSettings
  onError: (msg: string) => void
  onNote: (msg: string) => void
}

export default function OwnershipTab({ settings, onError, onNote }: OwnershipTabProps) {
  const [ownershipEntries, setOwnershipEntries] = useState<OwnershipEntry[]>([])
  const [busy, setBusy] = useState(false)
  const [newOwnershipRepo, setNewOwnershipRepo] = useState('')
  const [newOwnershipTeam, setNewOwnershipTeam] = useState('')
  const [newOwnershipDestIds, setNewOwnershipDestIds] = useState<string[]>([])
  const [searchQuery, setSearchQuery] = useState('')

  // Load on first open
  useEffect(() => {
    loadOwnershipEntries()
  }, [])

  const loadOwnershipEntries = async () => {
    try {
      const entries = await fetchOwnership()
      setOwnershipEntries(entries || [])
    } catch {
      // Silently fail; will show error on user action
    }
  }

  // All destination IDs across all channels for the picker
  const allDests: Array<{ id: string; name: string; kind: string }> = [
    ...(settings.slack.destinations ?? []).map((d) => ({ id: d.id, name: d.name, kind: 'slack' })),
    ...(settings.teams?.destinations ?? []).map((d) => ({ id: d.id, name: d.name, kind: 'teams' })),
    ...(settings.webhooks?.destinations ?? []).map((d) => ({ id: d.id, name: d.name, kind: 'webhook' })),
  ]

  async function handleSaveOwnership(e: React.FormEvent) {
    e.preventDefault()
    const repo = newOwnershipRepo.trim()
    const team = newOwnershipTeam.trim()

    if (!repo || !team) {
      onError('Repository and team name are required.')
      return
    }

    setBusy(true)
    onError('')
    onNote('')

    try {
      await upsertOwnership({ repository: repo, team_name: team, destination_ids: newOwnershipDestIds })
      await loadOwnershipEntries()
      setNewOwnershipRepo('')
      setNewOwnershipTeam('')
      setNewOwnershipDestIds([])
      onNote('Ownership entry saved.')
    } catch {
      onError('Failed to save ownership entry.')
    } finally {
      setBusy(false)
    }
  }

  async function handleRemoveOwnership(repo: string, team: string) {
    setBusy(true)
    try {
      await deleteOwnership(repo, team)
      await loadOwnershipEntries()
      onNote('Ownership entry removed.')
    } catch {
      onError('Failed to remove ownership entry.')
    } finally {
      setBusy(false)
    }
  }

  // Filter entries based on search query
  const filteredEntries = ownershipEntries.filter((entry) => {
    if (!searchQuery.trim()) return true
    const q = searchQuery.toLowerCase()
    return (
      entry.repository.toLowerCase().includes(q) ||
      entry.team_name.toLowerCase().includes(q) ||
      entry.destination_ids.some((id) => {
        const dest = allDests.find((d) => d.id === id)
        return dest?.name.toLowerCase().includes(q)
      })
    )
  })

  return (
    <div className="rr-card">
      <h2 className="font-serif text-[17px] font-bold text-[var(--text)] mb-1">Repository Ownership</h2>
      <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">
        Map repositories to team names and notification destinations. When an alert fires for a matched repository, RunRight
        automatically routes to these destinations in addition to any rule-configured ones.
      </p>

      {/* Form at the top */}
      <form className="settings-form mb-6" onSubmit={(e) => void handleSaveOwnership(e)}>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div className="form-group">
            <label>Repository</label>
            <input
              type="text"
              placeholder="owner/repo"
              value={newOwnershipRepo}
              onChange={(e) => setNewOwnershipRepo(e.target.value)}
              disabled={busy}
            />
          </div>
          <div className="form-group">
            <label>Team Name</label>
            <input
              type="text"
              placeholder="e.g. platform-team"
              value={newOwnershipTeam}
              onChange={(e) => setNewOwnershipTeam(e.target.value)}
              disabled={busy}
            />
          </div>
        </div>
        {allDests.length > 0 && (
          <div className="form-group">
            <label>Route alerts to</label>
            <div className="flex flex-wrap gap-2 mt-1">
              {allDests.map((d) => (
                <label key={d.id} className="flex items-center gap-1.5 text-sm cursor-pointer">
                  <input
                    type="checkbox"
                    checked={newOwnershipDestIds.includes(d.id)}
                    onChange={(e) =>
                      setNewOwnershipDestIds((prev) =>
                        e.target.checked ? [...prev, d.id] : prev.filter((id) => id !== d.id)
                      )
                    }
                    disabled={busy}
                  />
                  <span>
                    {d.name} <span className="text-[var(--text-light)] text-xs">({d.kind})</span>
                  </span>
                </label>
              ))}
            </div>
          </div>
        )}
        {allDests.length === 0 && (
          <p className="text-sm text-[var(--text-light)] mb-4">
            No destinations configured yet. Add a Slack destination first in the Destinations tab.
          </p>
        )}
        <button type="submit" className="btn-rr" disabled={busy || allDests.length === 0}>
          Save Ownership Rule
        </button>
      </form>

      {/* Search and entries list */}
      {ownershipEntries.length > 0 && (
        <>
          <div className="mb-4">
            <input
              type="text"
              placeholder="Search by repository, team, or destination..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full px-3 py-2 border border-[var(--border)] rounded bg-[var(--cream)] text-sm placeholder:text-[var(--text-light)] focus:outline-none focus:border-[var(--gold)]"
            />
          </div>
          <div className="divide-y divide-[var(--border)] border border-[var(--border)] rounded">
            {filteredEntries.length === 0 ? (
              <div className="px-4 py-6 text-center text-sm text-[var(--text-light)]">
                No ownership entries match your search.
              </div>
            ) : (
              filteredEntries.map((entry) => (
                <div key={`${entry.repository}:${entry.team_name}`} className="flex items-start justify-between px-4 py-3 gap-3">
                  <div className="min-w-0">
                    <div className="font-semibold text-sm text-[var(--text)] truncate">{entry.repository}</div>
                    <div className="text-xs text-[var(--text-mid)] mt-0.5">Team: {entry.team_name}</div>
                    <div className="text-xs text-[var(--text-light)] mt-0.5">
                      {entry.destination_ids.length === 0
                        ? 'No destinations'
                        : entry.destination_ids.map((id) => allDests.find((d) => d.id === id)?.name ?? id).join(', ')}
                    </div>
                  </div>
                  <button
                    type="button"
                    className="flex-shrink-0 text-xs font-deco tracking-widest text-[var(--red)] border border-[var(--red)] px-3 py-1 rounded hover:bg-[rgba(194,59,34,.06)]"
                    onClick={() => void handleRemoveOwnership(entry.repository, entry.team_name)}
                    disabled={busy}
                  >
                    Remove
                  </button>
                </div>
              ))
            )}
          </div>
        </>
      )}
    </div>
  )
}
