export const PAGE_SIZES = [10, 20, 50]

export type SortKey = 'latestDate' | 'medianCpu' | 'medianMem' | 'medianDuration' | 'avgCostDelta' | 'runCount'

export interface JobGroup {
  jobId: string
  repository: string
  ciPlatform: string
  provider: string
  detectedMachine: string
  suggestedMachine: string
  tier: string
  availableTiers: string[]
  runCount: number
  medianCpu: number
  medianMem: number
  medianDuration: number
  avgCostDelta: number
  avgSpotDelta: number
  spotRisk: string
  benefitLabel: string
  benefitTone: 'saving' | 'benefit' | 'neutral'
  monthlyCurrentSpend: number
  monthlySavings: number
  latestDate: string
}

export interface SavingsSnapshot {
  totalJobs: number
  jobsWithSavings: number
  estimatedCurrentMonthlySpend: number
  estimatedCurrentAnnualSpend: number
  estimatedMonthlySavings: number
  projectedAnnualSavings: number
  avgWastePercent: number
}
