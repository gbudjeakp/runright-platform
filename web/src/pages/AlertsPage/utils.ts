import { formatFromUSD } from '../../currency'
import type { CurrencyCode } from '../../currency'
import type { AlertRule, EventType } from './types'

export const metricLabel = (metric: AlertRule['metric']): string => {
  if (metric === 'max_cost_per_hour') return 'Cost/Hr'
  if (metric === 'waste_percent') return 'Waste %'
  return 'Savings Drop %'
}

export const eventLabel = (event: EventType): string => {
  if (event === 'policy_violation') return 'Policy Violation'
  if (event === 'high_waste') return 'High Waste'
  return 'Daily Summary'
}

export const eventDescription = (event: EventType): string => {
  if (event === 'policy_violation') return 'Fires when a job run costs more per hour than a limit set on the Policies page.'
  if (event === 'high_waste') return 'Fires when a job repeatedly uses less than 20% of its machine.'
  return 'Fires once per day with a summary of cost, waste, and policy events.'
}

export const thresholdUnit = (metric: AlertRule['metric']): string => {
  if (metric === 'max_cost_per_hour') return 'USD/hr'
  return '%'
}

export const thresholdPlaceholder = (metric: AlertRule['metric']): string => {
  if (metric === 'max_cost_per_hour') return '0.50'
  return '50'
}

export const thresholdStep = (metric: AlertRule['metric']): number => {
  if (metric === 'max_cost_per_hour') return 0.01
  return 1
}

export const thresholdHint = (metric: AlertRule['metric']): string => {
  if (metric === 'max_cost_per_hour') return 'Alert fires when job cost exceeds this hourly rate.'
  if (metric === 'waste_percent') return 'Alert fires when job waste exceeds this percentage.'
  return 'Alert fires when monthly savings drop more than this percentage.'
}

export const convertThresholdForMetricSwitch = (
  currentValue: string,
  fromMetric: AlertRule['metric'],
  toMetric: AlertRule['metric']
): string => {
  if (fromMetric === toMetric) return currentValue
  // When switching between non-cost metrics, keep value; when switching to/from cost, reset
  if ((fromMetric === 'max_cost_per_hour' && toMetric !== 'max_cost_per_hour') ||
      (fromMetric !== 'max_cost_per_hour' && toMetric === 'max_cost_per_hour')) {
    return toMetric === 'max_cost_per_hour' ? '0.50' : '50'
  }
  return currentValue
}

export const costInputDigits = (currency: CurrencyCode): number => {
  return currency === 'JPY' ? 0 : 2
}

export const displayCostFromUSD = (usdAmount: number, currency: CurrencyCode): string => {
  return formatFromUSD(usdAmount, currency, {
    minimumFractionDigits: costInputDigits(currency),
    maximumFractionDigits: costInputDigits(currency)
  })
}
