// Package notification provides a channel-agnostic notification system.
// Adding a new notification channel requires implementing the Channel interface
// and registering it in the Dispatcher's destination index.
package notification

import (
	"context"
	"fmt"
	"time"
)

// Channel is the single interface all notification adapters must satisfy.
// Implementations are responsible only for delivery; formatting is handled
// by the Dispatcher before calling Send.
type Channel interface {
	// Kind returns the canonical channel identifier (e.g. "slack", "teams", "webhook").
	Kind() string
	// Send delivers message to the destination. Returns an error if delivery fails.
	Send(ctx context.Context, message Message) error
}

// Message is the provider-agnostic payload passed to every Channel.
// Channels can use whichever fields are relevant to their format.
type Message struct {
	// Title is a short one-line summary (Teams/Webhook subject line).
	Title string
	// Body is the human-readable notification text (Slack-style markdown supported).
	Body string
	// Event is the originating event kind (e.g. "policy_violation").
	Event string
	// RuleID and RuleName identify which alert rule triggered this send.
	RuleID   string
	RuleName string
	// Scope metadata for context in messages.
	JobID      string
	Repository string
	// Extra key/value pairs adapters can pull for custom formatting.
	Meta map[string]string
	// SentAt is when the message was created (set by Dispatcher).
	SentAt time.Time
}

// DeliveryResult records one delivery attempt.
type DeliveryResult struct {
	DestinationID string
	Channel       string
	Status        string // "delivered" | "failed"
	Error         string
	SentAt        time.Time
	// RuleID and Payload are set by the Dispatcher so OnDelivery callbacks
	// have full context for retry scheduling and audit logging.
	RuleID  string
	Payload Message
}

// Destination pairs a Channel implementation with its identity and config.
type Destination struct {
	ID      string
	Name    string
	Channel Channel
}

// ErrDelivery wraps a delivery failure with context.
type ErrDelivery struct {
	DestinationID string
	Kind          string
	Cause         error
}

func (e *ErrDelivery) Error() string {
	return fmt.Sprintf("deliver to %s (%s): %v", e.DestinationID, e.Kind, e.Cause)
}

func (e *ErrDelivery) Unwrap() error { return e.Cause }
