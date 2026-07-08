export type RuleType = 'event' | 'threshold'
export type EventType = 'policy_violation' | 'high_waste' | 'daily_summary'

export interface AlertRule {
  id: string
  name: string
  type: RuleType
  // event rules
  event?: EventType
  // threshold rules
  scope: 'global' | 'repository' | 'job'
  repository: string
  jobId: string
  metric: 'max_cost_per_hour' | 'waste_percent' | 'monthly_savings_drop_percent'
  threshold: number
  destinationIds: string[]
  enabled: boolean
}

export interface ThresholdRuleDraft {
  name: string
  scope: 'global' | 'repository' | 'job'
  repository: string
  jobId: string
  metric: AlertRule['metric']
  threshold: string
  destinationIds: string[]
}

export interface EventRuleDraft {
  name: string
  event: EventType
  policyKey: string
  scope: 'global' | 'repository' | 'job'
  repository: string
  jobId: string
  destinationIds: string[]
}

export interface SlackDestination {
  id: string
  name: string
  webhook_url: string
  has_secret?: boolean
  channel: string
  mention: string
}

export interface TeamsDestination {
  id: string
  name: string
  webhook_url: string
  has_secret?: boolean
}

export interface WebhookDestination {
  id: string
  name: string
  url: string
  has_secret?: boolean
}

export interface EmailRecipient {
  email: string
  verified: boolean
}
