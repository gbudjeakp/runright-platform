package server

// notifier.go bridges the notification package to server.go's DB and
// settings storage. All notification dispatch goes through here.
//
// Adding a new channel type in the future:
//  1. Add an adapter in internal/notification/adapters.go implementing Channel.
//  2. Add a settings struct + DB secret handling in server.go (same as Teams).
//  3. Add a case in buildNotificationDispatcher below.
//  4. That's it — Dispatcher handles everything else.

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/sgbudje/runright-platform/internal/notification"
	"github.com/sgbudje/runright-platform/internal/types"
)

// dailySummaryStats holds aggregated stats for one daily summary window.
type dailySummaryStats struct {
	Runs            int
	AvgCPUP95       float64
	AvgMemGiBP95    float64
	AvgDurationSec  float64
	SavingsRunCount int
}

// dailySummaryLoop wakes up every hour and triggers daily summary dispatch at
// exactly 09:00 UTC.
func (s *Server) dailySummaryLoop() {
	for {
		now := time.Now().UTC()
		next := now.Truncate(time.Hour).Add(time.Hour)
		time.Sleep(time.Until(next))
		s.dispatchDailySummaries(next)
	}
}

// buildNotificationDispatcher constructs a notification.Dispatcher from the// current settings and the secret store. The Dispatcher is immutable per call;
// callers should not cache it across requests.
func (s *Server) buildNotificationDispatcher(ctx context.Context) (*notification.Dispatcher, []notification.Rule, notification.GlobalEvents, error) {
	settings, secrets, err := s.loadNotificationSettingsForDispatch(ctx)
	if err != nil {
		return nil, nil, notification.GlobalEvents{}, err
	}

	// Build channel-agnostic destination index.
	destinations := make(map[string]notification.Destination)

	for _, d := range settings.Slack.Destinations {
		url := strings.TrimSpace(secrets[d.ID])
		if url == "" {
			continue
		}
		destinations[d.ID] = notification.Destination{
			ID:   d.ID,
			Name: d.Name,
			Channel: &notification.SlackChannel{
				WebhookURL: url,
				Mention:    d.Mention,
			},
		}
	}
	for _, d := range settings.Teams.Destinations {
		url := strings.TrimSpace(secrets[d.ID])
		if url == "" {
			continue
		}
		destinations[d.ID] = notification.Destination{
			ID:   d.ID,
			Name: d.Name,
			Channel: &notification.TeamsChannel{WebhookURL: url},
		}
	}
	for _, d := range settings.Webhooks.Destinations {
		url := strings.TrimSpace(secrets[d.ID])
		if url == "" {
			continue
		}
		destinations[d.ID] = notification.Destination{
			ID:   d.ID,
			Name: d.Name,
			Channel: &notification.WebhookChannel{URL: url, Headers: d.Headers},
		}
	}

	// Add email destination if enabled with recipients
	if settings.Email.Enabled && len(settings.Email.Recipients) > 0 {
		destinations["email"] = notification.Destination{
			ID:   "email",
			Name: "Email",
			Channel: &notification.EmailChannel{
				SMTPHost:      s.smtpHost,
				SMTPUser:      s.smtpUser,
				SMTPPass:      s.smtpPass,
				FromAddress:   s.smtpFrom,
				Recipients:    settings.Email.Recipients,
				SubjectPrefix: settings.Email.SubjectPrefix,
			},
		}
	}

	// Convert server-side rule structs to notification.Rule.
	rules := make([]notification.Rule, 0, len(settings.Rules))
	for _, r := range settings.Rules {
		rules = append(rules, notification.Rule{
			ID:             r.ID,
			Name:           r.Name,
			Type:           notification.RuleType(r.Type),
			Event:          notification.EventKind(r.Event),
			Scope:          notification.RuleScope(r.Scope),
			Repository:     r.Repository,
			JobID:          r.JobID,
			Metric:         r.Metric,
			Threshold:      r.Threshold,
			DestinationIDs: r.DestinationIDs,
			Enabled:        r.Enabled,
		})
	}

	globalEvents := notification.GlobalEvents{
		PolicyViolation: settings.Events.PolicyViolation,
		HighWaste:       settings.Events.HighWaste,
		DailySummary:    settings.Events.DailySummary,
	}

	dispatcher := &notification.Dispatcher{
		Destinations: destinations,
		OnDelivery: func(result notification.DeliveryResult) {
			s.writeDeliveryLogWithRetry(context.Background(), result, "", "")
		},
	}

	return dispatcher, rules, globalEvents, nil
}

// dispatchNotificationRules is called after a job completes. It evaluates all
// enabled rules and routes matching events to their destinations.
func (s *Server) dispatchNotificationRules(summary types.MetricsSummary, recs []types.Recommendation) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	settings, secrets, err := s.loadNotificationSettingsForDispatch(ctx)
	if err != nil || !settings.Enabled {
		return
	}

	dispatcher, rules, globalEvents, err := s.buildNotificationDispatcher(ctx)
	if err != nil {
		return
	}
	_ = secrets // consumed inside buildNotificationDispatcher

	// Augment rules with ownership-based destinations for this repository.
	ownershipDestIDs, _ := s.loadOwnershipDestinations(ctx, summary.Repository)
	rules = mergeOwnershipIntoRules(rules, ownershipDestIDs)

	// Wire delivery log with retry scheduling.
	dispatcher.OnDelivery = func(result notification.DeliveryResult) {
		s.writeDeliveryLogWithRetry(ctx, result, summary.JobID, summary.Repository)
	}

	metrics := s.buildRuntimeMetrics(ctx, summary, recs)
	dispatcher.Dispatch(ctx, rules, globalEvents, metrics)
}

// dispatchDailySummaries runs at 09:00 UTC and dispatches any daily summary
// rules that haven't been sent today, scoped by the rule's configured scope.
func (s *Server) dispatchDailySummaries(now time.Time) {
	now = now.UTC()
	if now.Hour() != 9 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	settings, _, err := s.loadNotificationSettingsForDispatch(ctx)
	if err != nil || !settings.Enabled || !settings.Events.DailySummary {
		return
	}

	dispatcher, rules, globalEvents, err := s.buildNotificationDispatcher(ctx)
	if err != nil {
		return
	}

	day := now.Format("2006-01-02")

	for _, rule := range rules {
		if !rule.Enabled || rule.Type != notification.RuleTypeEvent || rule.Event != notification.EventDailySummary {
			continue
		}
		scopeKey := notificationScopeKey(rule)
		alreadySent, err := s.dailySummaryAlreadySent(ctx, rule.ID, scopeKey, day)
		if err != nil || alreadySent {
			continue
		}

		stats, err := s.computeDailySummaryStats(ctx, rule, now)
		if err != nil || stats.Runs == 0 {
			continue
		}

		notifStats := notification.DailySummaryStats{
			Runs:            stats.Runs,
			AvgCPUP95:       stats.AvgCPUP95,
			AvgMemGiBP95:    stats.AvgMemGiBP95,
			AvgDurationSec:  stats.AvgDurationSec,
			SavingsRunCount: stats.SavingsRunCount,
		}

		var delivered int
		origOnDelivery := dispatcher.OnDelivery
		dispatcher.OnDelivery = func(result notification.DeliveryResult) {
			if origOnDelivery != nil {
				origOnDelivery(result)
			}
			if result.Status == "delivered" {
				delivered++
			}
		}

		dispatcher.DispatchDailySummary(ctx, []notification.Rule{rule}, globalEvents, notifStats,
			rule.Scope, rule.Repository, rule.JobID, now)

		if delivered > 0 {
			_ = s.markDailySummarySent(ctx, rule.ID, scopeKey, day)
		}
	}
}

// buildRuntimeMetrics computes the runtime metrics for a completed job.
func (s *Server) buildRuntimeMetrics(ctx context.Context, summary types.MetricsSummary, recs []types.Recommendation) notification.RuntimeMetrics {
	detectedPrice := float64(0)
	if summary.DetectedMachine != nil {
		detectedPrice = summary.DetectedMachine.OnDemandPricePerHour
	}

	cpuUtil := summary.CPUPercentP95
	memUtil := 0.0
	if summary.MemTotalGiB > 0 {
		memUtil = (summary.MemUsedGiBP95 / summary.MemTotalGiB) * 100
	}
	util := cpuUtil
	if memUtil > util {
		util = memUtil
	}
	if util < 0 {
		util = 0
	}
	if util > 100 {
		util = 100
	}
	wastePercent := 100 - util

	metrics := notification.RuntimeMetrics{
		Repository:                summary.Repository,
		JobID:                     summary.JobID,
		DetectedPricePerHour:      detectedPrice,
		WastePercent:              wastePercent,
		MonthlySavingsDropPercent: s.computeSavingsDropPercent(ctx, summary.JobID, summary.Repository, recs),
	}

	if detectedPrice > 0 {
		policyEval := s.evaluateEffectivePolicy(ctx, summary.Repository, summary.JobID, detectedPrice)
		metrics.PolicyViolated = policyEval.Violated
		metrics.PolicyScope = policyEval.SourceScope
	}

	return metrics
}

// writeDeliveryLog writes one delivery attempt to the DB without retry scheduling.
// Used by test-dispatch paths where retry is not needed.
func (s *Server) writeDeliveryLog(ctx context.Context, result notification.DeliveryResult, jobID, repository string) {
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO notification_delivery_logs
			(rule_id, destination_id, channel, job_id, repository, status, error_message, attempts, max_attempts, sent_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 1, 4, NOW())`,
		"", result.DestinationID, result.Channel, jobID, repository, result.Status, result.Error)
}

// notificationScopeKey produces a stable dedupe key for a rule.
func notificationScopeKey(rule notification.Rule) string {
	switch rule.Scope {
	case notification.ScopeJob:
		return "job:" + rule.Repository + ":" + rule.JobID
	case notification.ScopeRepository:
		return "repository:" + rule.Repository
	default:
		return "global"
	}
}

// computeSavingsDropPercent computes the % drop in monthly savings vs the
// previous completed run for the same job. Returns 0 if no regression.
func (s *Server) computeSavingsDropPercent(ctx context.Context, jobID, repository string, recs []types.Recommendation) float64 {
	currentSavings := monthlySavingsFromRecommendations(recs)

	rows, err := s.db.QueryContext(ctx, `
		SELECT recommendations FROM jobs
		WHERE job_id = $1 AND COALESCE(repository, '') = $2 AND status = 'completed'
		ORDER BY created_at DESC
		LIMIT 2`, jobID, repository)
	if err != nil {
		return 0
	}
	defer rows.Close()

	index := 0
	previousSavings := 0.0
	for rows.Next() {
		var recsJSON json.RawMessage
		if err := rows.Scan(&recsJSON); err != nil {
			continue
		}
		if index == 1 {
			var prev []types.Recommendation
			if err := json.Unmarshal(recsJSON, &prev); err == nil {
				previousSavings = monthlySavingsFromRecommendations(prev)
			}
			break
		}
		index++
	}
	if previousSavings <= 0 || currentSavings >= previousSavings {
		return 0
	}
	return ((previousSavings - currentSavings) / previousSavings) * 100
}

func monthlySavingsFromRecommendations(recs []types.Recommendation) float64 {
	best := 0.0
	for _, rec := range recs {
		if rec.CostDeltaPercent < -0.5 {
			if saving := rec.CurrentMonthly - rec.EstimatedMonthly; saving > best {
				best = saving
			}
		}
	}
	return best
}

// computeDailySummaryStats runs a DB query to aggregate job metrics in a 24h
// window relative to now.
func (s *Server) computeDailySummaryStats(ctx context.Context, rule notification.Rule, now time.Time) (dailySummaryStats, error) {
	windowStart := now.Add(-24 * time.Hour)
	var rows *sql.Rows
	var err error
	switch rule.Scope {
	case notification.ScopeJob:
		rows, err = s.db.QueryContext(ctx, `
			SELECT summary, recommendations, duration_seconds FROM jobs
			WHERE status='completed' AND created_at>=$1 AND created_at<=$2
			  AND COALESCE(repository,'')=$3 AND job_id=$4`,
			windowStart, now, rule.Repository, rule.JobID)
	case notification.ScopeRepository:
		rows, err = s.db.QueryContext(ctx, `
			SELECT summary, recommendations, duration_seconds FROM jobs
			WHERE status='completed' AND created_at>=$1 AND created_at<=$2
			  AND COALESCE(repository,'')=$3`,
			windowStart, now, rule.Repository)
	default:
		rows, err = s.db.QueryContext(ctx, `
			SELECT summary, recommendations, duration_seconds FROM jobs
			WHERE status='completed' AND created_at>=$1 AND created_at<=$2`,
			windowStart, now)
	}
	if err != nil {
		return dailySummaryStats{}, err
	}
	defer rows.Close()

	stats := dailySummaryStats{}
	for rows.Next() {
		var summaryJSON, recsJSON json.RawMessage
		var dur float64
		if err := rows.Scan(&summaryJSON, &recsJSON, &dur); err != nil {
			continue
		}
		var sm types.MetricsSummary
		if err := json.Unmarshal(summaryJSON, &sm); err == nil {
			stats.AvgCPUP95 += sm.CPUPercentP95
			stats.AvgMemGiBP95 += sm.MemUsedGiBP95
		}
		stats.AvgDurationSec += dur
		var recs []types.Recommendation
		if err := json.Unmarshal(recsJSON, &recs); err == nil && monthlySavingsFromRecommendations(recs) > 0 {
			stats.SavingsRunCount++
		}
		stats.Runs++
	}
	if rows.Err() != nil {
		return dailySummaryStats{}, rows.Err()
	}
	if stats.Runs > 0 {
		d := float64(stats.Runs)
		stats.AvgCPUP95 /= d
		stats.AvgMemGiBP95 /= d
		stats.AvgDurationSec /= d
	}
	return stats, nil
}
