package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// SlackChannel delivers messages to a Slack Incoming Webhook.
type SlackChannel struct {
	WebhookURL string
	Mention    string // optional @mention prepended to every message
}

func (c *SlackChannel) Kind() string { return "slack" }

func (c *SlackChannel) Send(ctx context.Context, msg Message) error {
	text := msg.Body
	if c.Mention != "" {
		text = c.Mention + " " + text
	}
	payload := map[string]string{"text": text}
	return postJSON(ctx, c.WebhookURL, payload, nil)
}

// TeamsChannel delivers messages to a Microsoft Teams Incoming Webhook
// using the MessageCard format (universally supported).
type TeamsChannel struct {
	WebhookURL string
}

func (c *TeamsChannel) Kind() string { return "teams" }

func (c *TeamsChannel) Send(ctx context.Context, msg Message) error {
	payload := map[string]any{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": "C23B22",
		"summary":    msg.Title,
		"sections": []map[string]any{
			{
				"activityTitle": msg.Title,
				"activityText":  msg.Body,
				"markdown":      true,
			},
		},
	}
	return postJSON(ctx, c.WebhookURL, payload, nil)
}

// WebhookChannel delivers structured JSON payloads to any HTTP endpoint.
// It is the escape hatch for custom integrations (PagerDuty, custom pipelines, etc.).
type WebhookChannel struct {
	URL     string
	Headers map[string]string // e.g. Authorization, X-Custom-Header
}

func (c *WebhookChannel) Kind() string { return "webhook" }

func (c *WebhookChannel) Send(ctx context.Context, msg Message) error {
	payload := map[string]any{
		"rule_id":    msg.RuleID,
		"rule_name":  msg.RuleName,
		"event":      msg.Event,
		"title":      msg.Title,
		"body":       msg.Body,
		"job_id":     msg.JobID,
		"repository": msg.Repository,
		"meta":       msg.Meta,
		"sent_at":    msg.SentAt.Format(time.RFC3339),
	}
	return postJSON(ctx, c.URL, payload, c.Headers)
}

// EmailChannel delivers notifications via SMTP email.
type EmailChannel struct {
	SMTPHost      string   // e.g. "smtp.example.com:587"
	SMTPUser      string   // SMTP username for auth
	SMTPPass      string   // SMTP password for auth
	FromAddress   string   // e.g. "alerts@runright.io"
	Recipients    []string // destination email addresses
	SubjectPrefix string   // e.g. "[RunRight]"
}

func (c *EmailChannel) Kind() string { return "email" }

func (c *EmailChannel) Send(ctx context.Context, msg Message) error {
	if len(c.Recipients) == 0 {
		return fmt.Errorf("no email recipients configured")
	}
	if c.SMTPHost == "" {
		// If no SMTP configured, log to console (demo mode)
		fmt.Printf("[EMAIL DEMO] Would send to %v: %s - %s\n", c.Recipients, msg.Title, msg.Body)
		return nil
	}

	subject := msg.Title
	if c.SubjectPrefix != "" {
		subject = c.SubjectPrefix + " " + subject
	}

	// Build plain-text email message
	var body strings.Builder
	body.WriteString(fmt.Sprintf("From: %s\r\n", c.FromAddress))
	body.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(c.Recipients, ", ")))
	body.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	body.WriteString("MIME-Version: 1.0\r\n")
	body.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n")
	body.WriteString("\r\n")
	body.WriteString(msg.Body)
	body.WriteString("\r\n\r\n---\r\n")
	body.WriteString(fmt.Sprintf("Event: %s\r\n", msg.Event))
	if msg.Repository != "" {
		body.WriteString(fmt.Sprintf("Repository: %s\r\n", msg.Repository))
	}
	if msg.JobID != "" {
		body.WriteString(fmt.Sprintf("Job ID: %s\r\n", msg.JobID))
	}
	body.WriteString(fmt.Sprintf("Rule: %s\r\n", msg.RuleName))
	body.WriteString(fmt.Sprintf("Sent: %s\r\n", msg.SentAt.Format(time.RFC3339)))

	// Use SMTP auth if credentials provided
	var auth smtp.Auth
	if c.SMTPUser != "" && c.SMTPPass != "" {
		host := strings.Split(c.SMTPHost, ":")[0]
		auth = smtp.PlainAuth("", c.SMTPUser, c.SMTPPass, host)
	}

	err := smtp.SendMail(c.SMTPHost, auth, c.FromAddress, c.Recipients, []byte(body.String()))
	if err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}

// postJSON marshals payload and POSTs it, applying optional extra headers.
func postJSON(ctx context.Context, url string, payload any, headers map[string]string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}
