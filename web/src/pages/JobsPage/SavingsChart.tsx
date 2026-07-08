import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from 'recharts'
import { formatFromUSD } from '../../currency'
import type { CurrencyCode } from '../../currency'
import type { SavingsHistoryPoint } from '../../types'
import { formatShortDate } from './utils'

interface SavingsChartProps {
  history: SavingsHistoryPoint[]
  currency: CurrencyCode
}

export default function SavingsChart({ history, currency }: SavingsChartProps) {
  if (history.length < 2) return null

  return (
    <div className="rr-card !mb-5 !p-4">
      <div className="font-deco text-[12px] tracking-[2px] mb-3" style={{ color: '#C4A882' }}>
        SAVINGS OVER TIME (90 DAYS)
      </div>
      <ResponsiveContainer width="100%" height={160}>
        <LineChart data={history} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
          <CartesianGrid strokeDasharray="3 3" stroke="#3a2510" />
          <XAxis
            dataKey="date"
            tick={{ fontSize: 10, fill: '#9A7B5A' }}
            tickFormatter={(d: string) => formatShortDate(d)}
          />
          <YAxis
            tick={{ fontSize: 10, fill: '#9A7B5A' }}
            tickFormatter={(v: number) =>
              formatFromUSD(v, currency, { minimumFractionDigits: 0, maximumFractionDigits: 0 })
            }
            width={80}
          />
          <Tooltip
            contentStyle={{ background: '#2C1A0E', border: '1px solid #3a2510', fontSize: 12 }}
            labelFormatter={(label) => formatShortDate(String(label))}
            formatter={(v) => [`${formatFromUSD(Number(v ?? 0), currency)}/mo`, 'Potential saving']}
          />
          <Line type="monotone" dataKey="monthly_savings" stroke="#E8C458" dot={false} strokeWidth={2} />
        </LineChart>
      </ResponsiveContainer>
    </div>
  )
}
