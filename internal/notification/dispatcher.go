package notification

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// EventKind is a typed event identifier.
type EventKind string

const (
	EventPolicyViolation EventKind = "policy_violation"
	EventHighWaste       EventKind = "high_waste"
	EventDailySummary    EventKind = "daily_summary"
	EventThreshold       EventKind = "threshold"
)

// RuleType distinguishes event-triggered from threshold-triggered rules.
type RuleType string

const (
	RuleTypeEvent     RuleType = "event"
	RuleTypeThreshold RuleType = "threshold"
)

// RuleScope controls which jobs a rule applies to.
type RuleScope string

const (
	ScopeGlobal     RuleScope = "global"
	ScopeRepository RuleScope = "repository"
	ScopeJob        RuleScope = "job"
)

// Rule is the provider-agnostic representation of an alert rule.
type Rule struct {
	ID             string
	Name           string
	Type           RuleType
	Event          EventKind
	Scope          RuleScope
	Repository     string
	JobID          string
	Metric         string
	Threshold      float64
	DestinationIDs []string
	Enabled        bool
}

// RuntimeMetrics carries the computed metrics for a completed job needed
// to evaluate whether any rule should fire.
type RuntimeMetrics struct {
	Repository                string
	JobID                     string
	DetectedPricePerHour      float64
	WastePercent              float64
	MonthlySavingsDropPercent float64
	PolicyViolated            bool
	PolicyScope               string
}

// GlobalEvents holds the event-level toggles (mirrors notificationEvents in server).
type GlobalEvents struct {
	PolicyViolation bool
	HighWaste       bool
	DailySummary    bool
}

// DailySummaryStats carries aggregated stats for a daily summary message.
type DailySummaryStats struct {
	Runs            int
	AvgCPUP95       float64
	AvgMemGiBP95    float64
	AvgDurationSec  float64
	SavingsRunCount int
}

// Dispatcher evaluates rules against runtime metrics and delivers messages
// to the appropriate channels using the provided destination index.
type Dispatcher struct {
	// Destinations maps destination ID → ready-to-use Destination.
	Destinations map[string]Destination
	// OnDelivery is called after each delivery attempt (success or failure).
	// Use it to record delivery logs. May be nil.
	OnDelivery func(result DeliveryResult)
}

// Dispatch evaluates every rule in rules against metrics, then sends to
// all matching destinations. Rules are evaluated independently; a single
// job completing can trigger multiple rules to multiple channels.
func (d *Dispatcher) Dispatch(ctx context.Context, rules []Rule, events GlobalEvents, metrics RuntimeMetrics) {
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if !scopeMatches(rule, metrics.Repository, metrics.JobID) {
			continue
		}
		shouldSend, value, threshold := d.shouldTrigger(rule, events, metrics)
		if !shouldSend {
			continue
		}
		msg := d.formatMessage(rule, metrics, value, threshold)
		d.send(ctx, rule, msg)
	}
}

// DispatchDailySummary sends daily summary messages for rules that match
// the event=daily_summary type. It is called by the scheduler, not per-job.
func (d *Dispatcher) DispatchDailySummary(ctx context.Context, rules []Rule, events GlobalEvents, stats DailySummaryStats, scope RuleScope, repository, jobID string, windowEnd time.Time) {
	if !events.DailySummary {
		return
	}
	for _, rule := range rules {
		if !rule.Enabled || rule.Type != RuleTypeEvent || rule.Event != EventDailySummary {
			continue
		}
		if rule.Scope != scope {
			continue
		}
		if scope == ScopeRepository && rule.Repository != repository {
			continue
		}
		if scope == ScopeJob && (rule.Repository != repository || rule.JobID != jobID) {
			continue
		}
		msg := d.formatDailySummaryMessage(rule, stats, windowEnd)
		d.send(ctx, rule, msg)
	}
}

func (d *Dispatcher) send(ctx context.Context, rule Rule, msg Message) {
	for _, destID := range rule.DestinationIDs {
		dest, ok := d.Destinations[destID]
		if !ok {
			continue
		}
		err := dest.Channel.Send(ctx, msg)
		if d.OnDelivery != nil {
			result := DeliveryResult{
				DestinationID: dest.ID,
				Channel:       dest.Channel.Kind(),
				SentAt:        time.Now().UTC(),
				RuleID:        rule.ID,
				Payload:       msg,
			}
			if err != nil {
				result.Status = "failed"
				result.Error = err.Error()
			} else {
				result.Status = "delivered"
			}
			d.OnDelivery(result)
		}
	}
}

func (d *Dispatcher) shouldTrigger(rule Rule, events GlobalEvents, metrics RuntimeMetrics) (bool, float64, float64) {
	switch rule.Type {
	case RuleTypeEvent:
		switch rule.Event {
		case EventPolicyViolation:
			return events.PolicyViolation && metrics.PolicyViolated, metrics.DetectedPricePerHour, 0
		case EventHighWaste:
			return events.HighWaste && metrics.WastePercent >= 80, metrics.WastePercent, 80
		case EventDailySummary:
			return false, 0, 0 // handled separately by DispatchDailySummary
		}
	case RuleTypeThreshold:
		var value float64
		switch rule.Metric {
		case "max_cost_per_hour":
			value = metrics.DetectedPricePerHour
		case "waste_percent":
			value = metrics.WastePercent
		case "monthly_savings_drop_percent":
			value = metrics.MonthlySavingsDropPercent
		}
		return value >= rule.Threshold, value, rule.Threshold
	}
	return false, 0, 0
}

func scopeMatches(rule Rule, repository, jobID string) bool {
	switch rule.Scope {
	case ScopeGlobal:
		return true
	case ScopeRepository:
		return rule.Repository == repository
	case ScopeJob:
		return rule.Repository == repository && rule.JobID == jobID
	}
	return false
}

func (d *Dispatcher) formatMessage(rule Rule, metrics RuntimeMetrics, value, threshold float64) Message {
	repo := metrics.Repository
	if repo == "" {
		repo = "local"
	}
	jobID := metrics.JobID
	if jobID == "" {
		jobID = "unknown"
	}

	var title, body string
	switch rule.Type {
	case RuleTypeEvent:
		switch rule.Event {
		case EventPolicyViolation:
			scope := metrics.PolicyScope
			if scope == "" || scope == "none" {
				scope = "configured"
			}
			title = fmt.Sprintf("RunRight policy violation — %s", rule.Name)
			body = fmt.Sprintf(":rotating_light: *Policy violation* (%s scope)\n• Rule: %s\n• Repository: `%s`\n• Job: `%s`\n• Detected cost/hour: $%.4f", scope, rule.Name, repo, jobID, value)
		case EventHighWaste:
			title = fmt.Sprintf("RunRight high waste — %s", rule.Name)
			body = fmt.Sprintf(":warning: *High waste detected*\n• Rule: %s\n• Repository: `%s`\n• Job: `%s`\n• Waste: %.1f%%", rule.Name, repo, jobID, value)
		default:
			title = fmt.Sprintf("RunRight alert — %s", rule.Name)
			body = fmt.Sprintf(":information_source: %s\n• Repository: `%s`\n• Job: `%s`", rule.Name, repo, jobID)
		}
	case RuleTypeThreshold:
		metricLabel := strings.ReplaceAll(rule.Metric, "_", " ")
		title = fmt.Sprintf("RunRight threshold alert — %s", rule.Name)
		body = fmt.Sprintf(":bell: *Threshold exceeded*\n• Rule: %s\n• Metric: %s\n• Value: %.2f (threshold: %.2f)\n• Repository: `%s`\n• Job: `%s`", rule.Name, metricLabel, value, threshold, repo, jobID)
	}

	return Message{
		Title:      title,
		Body:       body,
		Event:      string(rule.Event),
		RuleID:     rule.ID,
		RuleName:   rule.Name,
		JobID:      jobID,
		Repository: repo,
		SentAt:     time.Now().UTC(),
	}
}

func (d *Dispatcher) formatDailySummaryMessage(rule Rule, stats DailySummaryStats, windowEnd time.Time) Message {
	scope := "Global"
	switch rule.Scope {
	case ScopeRepository:
		scope = fmt.Sprintf("Repository `%s`", rule.Repository)
	case ScopeJob:
		scope = fmt.Sprintf("Job `%s` in `%s`", rule.JobID, rule.Repository)
	}

	title := fmt.Sprintf("RunRight daily summary — %s", rule.Name)
	body := fmt.Sprintf(
		":calendar: *Daily summary* (%s)\n• Rule: %s\n• Window: last 24h ending %s UTC\n• Completed runs: %d\n• Avg p95 CPU: %.1f%%\n• Avg p95 memory: %.2f GiB\n• Avg duration: %.0fs\n• Runs with cheaper option: %d",
		scope, rule.Name, windowEnd.Format("2006-01-02 15:04"),
		stats.Runs, stats.AvgCPUP95, stats.AvgMemGiBP95, stats.AvgDurationSec, stats.SavingsRunCount,
	)

	return Message{
		Title:    title,
		Body:     body,
		Event:    string(EventDailySummary),
		RuleID:   rule.ID,
		RuleName: rule.Name,
		SentAt:   time.Now().UTC(),
	}
}
