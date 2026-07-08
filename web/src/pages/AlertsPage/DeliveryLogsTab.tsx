import { useEffect, useState } from 'react'
import { fetchDeliveryLogs } from '../../api'
import type { DeliveryLog } from '../../types'

export interface DeliveryLogsTabProps {
  onError?: (msg: string) => void
}

export default function DeliveryLogsTab({ onError }: DeliveryLogsTabProps) {
  const [deliveryLogs, setDeliveryLogs] = useState<DeliveryLog[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    refreshLogs()
  }, [])

  const refreshLogs = async () => {
    try {
      setLoading(true)
      const logs = await fetchDeliveryLogs(undefined, 100)
      setDeliveryLogs(logs || [])
    } catch (err) {
      onError?.('Failed to load delivery logs')
      setDeliveryLogs([])
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="rr-card">
      <h2 className="font-serif text-[17px] font-bold text-[var(--text)] mb-1">Delivery Logs</h2>
      <p className="text-sm text-[var(--text-light)] leading-relaxed mb-5">Recent notification delivery attempts. Reload to refresh.</p>
      <button type="button" className="btn-rr-sm mb-5" onClick={refreshLogs} disabled={loading}>
        {loading ? 'Loading…' : 'Refresh'}
      </button>

      {loading && <p className="text-sm text-[var(--text-light)]">Loading…</p>}

      {!loading && deliveryLogs.length === 0 && (
        <p className="text-sm text-[var(--text-light)]">No delivery logs yet. Deliveries appear here once alert rules fire.</p>
      )}

      {deliveryLogs.length > 0 && (
        <div className="overflow-x-auto">
          <table className="w-full text-xs border-collapse">
            <thead>
              <tr className="border-b border-[var(--border)] text-left">
                {['Status', 'Channel', 'Destination', 'Rule', 'Job', 'Repository', 'Sent'].map((h) => (
                  <th key={h} className="py-2 pr-4 font-deco tracking-widest text-[var(--text-light)] whitespace-nowrap">
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {deliveryLogs.map((log) => (
                <tr key={log.id} className="border-b border-[var(--border)] hover:bg-[var(--cream-alt)]">
                  <td className="py-2 pr-4">
                    <span
                      className={`inline-block px-2 py-0.5 rounded text-[10px] font-deco tracking-wider ${
                        log.status === 'delivered'
                          ? 'bg-[rgba(46,125,50,.10)] text-[#2E7D32]'
                          : 'bg-[rgba(194,59,34,.10)] text-[var(--red)]'
                      }`}
                    >
                      {log.status}
                    </span>
                  </td>
                  <td className="py-2 pr-4 text-[var(--text-mid)]">{log.channel}</td>
                  <td className="py-2 pr-4 text-[var(--text-mid)] max-w-[120px] truncate">{log.destination_id}</td>
                  <td className="py-2 pr-4 text-[var(--text-mid)] max-w-[120px] truncate">{log.rule_id || '—'}</td>
                  <td className="py-2 pr-4 text-[var(--text-mid)] max-w-[100px] truncate">{log.job_id || '—'}</td>
                  <td className="py-2 pr-4 text-[var(--text-mid)] max-w-[120px] truncate">{log.repository || '—'}</td>
                  <td className="py-2 pr-4 text-[var(--text-light)] whitespace-nowrap">{new Date(log.sent_at).toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
