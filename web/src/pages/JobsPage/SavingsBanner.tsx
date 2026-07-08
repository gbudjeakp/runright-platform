import { formatFromUSD } from '../../currency'
import type { CurrencyCode } from '../../currency'
import type { SavingsSnapshot } from './types'

interface SavingsBannerProps {
  snapshot: SavingsSnapshot
  currency: CurrencyCode
}

export default function SavingsBanner({ snapshot, currency }: SavingsBannerProps) {
  return (
    <div
      className="flex flex-wrap gap-6 sm:gap-8 items-center rounded-md px-5 py-4 mb-5"
      style={{ background: 'linear-gradient(90deg,#2C1A0E,#3D2510)', color: '#FBF0DC' }}
    >
      <div>
        <div className="font-deco text-[11px] tracking-[2px] mb-0.5" style={{ color: '#C4A882' }}>
          EST. CURRENT MONTHLY SPEND
        </div>
        <div className="font-deco text-3xl" style={{ color: '#FBF0DC' }}>
          {formatFromUSD(snapshot.estimatedCurrentMonthlySpend, currency)}
        </div>
      </div>
      <div>
        <div className="font-deco text-[11px] tracking-[2px] mb-0.5" style={{ color: '#C4A882' }}>
          EST. CURRENT ANNUAL SPEND
        </div>
        <div className="font-deco text-3xl" style={{ color: '#FBF0DC' }}>
          {formatFromUSD(snapshot.estimatedCurrentAnnualSpend, currency, {
            minimumFractionDigits: 0,
            maximumFractionDigits: 0,
          })}
        </div>
      </div>
      <div>
        <div className="font-deco text-[11px] tracking-[2px] mb-0.5" style={{ color: '#C4A882' }}>
          POTENTIAL MONTHLY SAVINGS
        </div>
        <div className="font-deco text-3xl" style={{ color: '#E8C458' }}>
          {formatFromUSD(snapshot.estimatedMonthlySavings, currency)}
        </div>
      </div>
      <div>
        <div className="font-deco text-[11px] tracking-[2px] mb-0.5" style={{ color: '#C4A882' }}>
          POTENTIAL ANNUAL SAVINGS
        </div>
        <div className="font-deco text-3xl" style={{ color: '#FBF0DC' }}>
          {formatFromUSD(snapshot.projectedAnnualSavings, currency, {
            minimumFractionDigits: 0,
            maximumFractionDigits: 0,
          })}
        </div>
      </div>
      <div>
        <div className="font-deco text-[11px] tracking-[2px] mb-0.5" style={{ color: '#C4A882' }}>
          OVER-PROVISIONED JOBS
        </div>
        <div className="font-deco text-3xl">
          {snapshot.jobsWithSavings}{' '}
          <span className="font-deco text-sm" style={{ color: '#C4A882' }}>
            of {snapshot.totalJobs}
          </span>
        </div>
      </div>
      <div>
        <div className="font-deco text-[11px] tracking-[2px] mb-0.5" style={{ color: '#C4A882' }}>
          AVG OVER-PROVISION
        </div>
        <div className="font-deco text-3xl text-red">{snapshot.avgWastePercent.toFixed(1)}%</div>
      </div>
    </div>
  )
}
