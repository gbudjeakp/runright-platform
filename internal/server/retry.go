package server

// retry.go — Postgres-backed retry queue for failed notification deliveries.
//
// How it works:
//  1. writeDeliveryLog records every attempt. For failures it also sets
//     next_retry_at using exponential backoff and increments attempts.
//  2. retryLoop polls every 30 s for failed rows that are due for retry.
//  3. Each retry re-loads the destination secret (so rotated secrets work),
//     replays the stored message payload, and updates the log row.
//  4. Rows that reach max_attempts are marked "permanently_failed" and
//     next_retry_at is cleared — they act as the dead letter.

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/sgbudje/runright-platform/internal/notification"
)

// retryBackoffDurations controls the wait time before each retry attempt.
// Attempt 1 (first failure) → 1 min, attempt 2 → 5 min, attempt 3 → 30 min.
// Attempt 4+ → permanently failed (no further retries).
var retryBackoffDurations = []time.Duration{
	1 * time.Minute,
	5 * time.Minute,
	30 * time.Minute,
}

const retryMaxAttempts = 4 // 1 initial + 3 retries

// writeDeliveryLogWithRetry records one delivery attempt and, if it failed,
// schedules a retry by computing next_retry_at from the backoff table.
// ruleID and the message payload are read from result.RuleID / result.Payload.
func (s *Server) writeDeliveryLogWithRetry(ctx context.Context, result notification.DeliveryResult, jobID, repository string) {
	payloadJSON, _ := json.Marshal(result.Payload)
	payloadStr := string(payloadJSON)

	if result.Status == "delivered" {
		_, _ = s.db.ExecContext(ctx, `
			INSERT INTO notification_delivery_logs
				(rule_id, destination_id, channel, job_id, repository,
				 status, error_message, attempts, max_attempts, payload, sent_at)
			VALUES ($1,$2,$3,$4,$5,'delivered','',$6,$7,$8,NOW())`,
			result.RuleID, result.DestinationID, result.Channel, jobID, repository,
			1, retryMaxAttempts, payloadStr)
		return
	}

	// Failed — schedule first retry if we haven't exceeded max attempts.
	nextRetry := time.Now().UTC().Add(retryBackoffDurations[0])
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO notification_delivery_logs
			(rule_id, destination_id, channel, job_id, repository,
			 status, error_message, attempts, max_attempts, next_retry_at, payload, sent_at)
		VALUES ($1,$2,$3,$4,$5,'failed',$6,1,$7,$8,$9,NOW())`,
		result.RuleID, result.DestinationID, result.Channel, jobID, repository,
		result.Error, retryMaxAttempts, nextRetry, payloadStr)
}

// retryLoop polls the delivery log every 30 s for failed rows that are due for
// retry and attempts redelivery. It runs as a background goroutine.
func (s *Server) retryLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.processRetries()
	}
}

type retryRow struct {
	id            int
	ruleID        string
	destinationID string
	channel       string
	jobID         string
	repository    string
	attempts      int
	maxAttempts   int
	payload       string
}

func (s *Server) processRetries() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, rule_id, destination_id, channel,
		       COALESCE(job_id,''), COALESCE(repository,''),
		       attempts, max_attempts, COALESCE(payload,'')
		FROM notification_delivery_logs
		WHERE status = 'failed'
		  AND next_retry_at IS NOT NULL
		  AND next_retry_at <= NOW()
		  AND attempts < max_attempts
		ORDER BY next_retry_at ASC
		LIMIT 50`)
	if err != nil {
		return
	}
	defer rows.Close()

	var pending []retryRow
	for rows.Next() {
		var r retryRow
		if err := rows.Scan(&r.id, &r.ruleID, &r.destinationID, &r.channel,
			&r.jobID, &r.repository, &r.attempts, &r.maxAttempts, &r.payload); err != nil {
			continue
		}
		pending = append(pending, r)
	}
	_ = rows.Close()

	if len(pending) == 0 {
		return
	}

	// Build dispatcher once for this batch.
	dispatcher, _, _, err := s.buildNotificationDispatcher(ctx)
	if err != nil {
		return
	}

	for _, row := range pending {
		s.retryDelivery(ctx, dispatcher, row)
	}
}

func (s *Server) retryDelivery(ctx context.Context, dispatcher *notification.Dispatcher, row retryRow) {
	dest, ok := dispatcher.Destinations[row.destinationID]
	if !ok {
		// Destination removed — permanently fail this row.
		s.markRetryExhausted(ctx, row.id, "destination no longer configured")
		return
	}

	var msg notification.Message
	if err := json.Unmarshal([]byte(row.payload), &msg); err != nil {
		s.markRetryExhausted(ctx, row.id, "payload unmarshal failed: "+err.Error())
		return
	}

	// Deliver via channel.
	deliveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	deliveryErr := dest.Channel.Send(deliveryCtx, msg)
	cancel()

	newAttempts := row.attempts + 1
	if deliveryErr == nil {
		// Success — mark delivered.
		_, _ = s.db.ExecContext(ctx, `
			UPDATE notification_delivery_logs
			SET status='delivered', error_message='', next_retry_at=NULL, attempts=$1
			WHERE id=$2`, newAttempts, row.id)
		return
	}

	// Still failing.
	if newAttempts >= row.maxAttempts {
		s.markRetryExhausted(ctx, row.id, deliveryErr.Error())
		return
	}

	// Schedule next retry with backoff.
	backoffIdx := newAttempts - 1
	if backoffIdx >= len(retryBackoffDurations) {
		backoffIdx = len(retryBackoffDurations) - 1
	}
	nextRetry := time.Now().UTC().Add(retryBackoffDurations[backoffIdx])
	_, _ = s.db.ExecContext(ctx, `
		UPDATE notification_delivery_logs
		SET attempts=$1, error_message=$2, next_retry_at=$3
		WHERE id=$4`,
		newAttempts, strings.TrimSpace(deliveryErr.Error()), nextRetry, row.id)
}

func (s *Server) markRetryExhausted(ctx context.Context, id int, reason string) {
	_, _ = s.db.ExecContext(ctx, `
		UPDATE notification_delivery_logs
		SET status='permanently_failed', error_message=$1, next_retry_at=NULL
		WHERE id=$2`, reason, id)
}
