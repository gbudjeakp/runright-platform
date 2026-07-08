package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	promexporter "go.opentelemetry.io/otel/exporters/prometheus"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sgbudje/runright-platform/internal/types"

	"net"
)

// Backend is a named export destination.
type Backend string

const (
	BackendFile       Backend = "file"
	BackendOTLP       Backend = "otlp"
	BackendPrometheus Backend = "prometheus"
	BackendHTTP       Backend = "http"
	BackendSlack      Backend = "slack"
	BackendTeams      Backend = "teams"
)

// ParseBackends parses a comma-separated list of backend names.
func ParseBackends(s string) []Backend {
	var out []Backend
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(strings.ToLower(part))
		switch Backend(part) {
		case BackendFile, BackendOTLP, BackendPrometheus, BackendHTTP, BackendSlack, BackendTeams:
			out = append(out, Backend(part))
		}
	}
	if len(out) == 0 {
		out = []Backend{BackendFile}
	}
	return out
}

// Manager holds active OTEL providers for configured backends.
type Manager struct {
	backends     []Backend
	provider     *sdkmetric.MeterProvider
	meter        otelmetric.Meter
	promPort     int
	httpURL      string
	slackWebhook string
	teamsWebhook string
	promLn       net.Listener
}

// New initialises the export manager. Call Shutdown when done.
func New(ctx context.Context, backends []Backend, promPort int, httpURL string, slackWebhook string, teamsWebhook string) (*Manager, error) {
	m := &Manager{
		backends:     backends,
		promPort:     promPort,
		httpURL:      httpURL,
		slackWebhook: slackWebhook,
		teamsWebhook: teamsWebhook,
	}

	var readers []sdkmetric.Reader

	for _, b := range backends {
		switch b {
		case BackendOTLP:
			exp, err := otlpmetricgrpc.New(ctx)
			if err != nil {
				return nil, fmt.Errorf("otlp exporter: %w", err)
			}
			readers = append(readers, sdkmetric.NewPeriodicReader(exp,
				sdkmetric.WithInterval(10*time.Second)))

		case BackendPrometheus:
			exp, err := promexporter.New()
			if err != nil {
				return nil, fmt.Errorf("prometheus exporter: %w", err)
			}
			readers = append(readers, exp)
			// Start minimal HTTP server for /metrics scraping.
			ln, err := net.Listen("tcp", fmt.Sprintf(":%d", promPort))
			if err != nil {
				return nil, fmt.Errorf("prometheus listener: %w", err)
			}
			m.promLn = ln
			mux := http.NewServeMux()
			mux.Handle("/metrics", promhttp.Handler())
			srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
			go func() { _ = srv.Serve(ln) }()
		}
	}

	if len(readers) > 0 {
		res, _ := resource.New(ctx,
			resource.WithAttributes(
				semconv.ServiceNameKey.String("runright"),
				attribute.String("service.version", "dev"),
			),
		)
		mpOpts := []sdkmetric.Option{sdkmetric.WithResource(res)}
		for _, r := range readers {
			mpOpts = append(mpOpts, sdkmetric.WithReader(r))
		}
		mp := sdkmetric.NewMeterProvider(mpOpts...)
		m.provider = mp
		m.meter = mp.Meter("runright")
	}

	return m, nil
}

// RecordSnapshot publishes a single metric snapshot to all OTEL-capable backends.
func (m *Manager) RecordSnapshot(ctx context.Context, snap types.MetricSnapshot) {
	if m.meter == nil {
		return
	}
	attrs := otelmetric.WithAttributes(attribute.String("job", snap.Timestamp.Format(time.RFC3339)))

	cpuGauge, _ := m.meter.Float64Gauge("runright.cpu.percent",
		otelmetric.WithDescription("CPU usage percentage"),
		otelmetric.WithUnit("%"))
	cpuGauge.Record(ctx, snap.CPUPercent, attrs)

	memGauge, _ := m.meter.Float64Gauge("runright.memory.used_gib",
		otelmetric.WithDescription("Memory used in GiB"))
	memGauge.Record(ctx, snap.MemUsedGiB, attrs)

	procGauge, _ := m.meter.Int64Gauge("runright.process.count",
		otelmetric.WithDescription("Number of running processes"))
	procGauge.Record(ctx, int64(snap.ProcessCount), attrs)

	threadGauge, _ := m.meter.Int64Gauge("runright.thread.count",
		otelmetric.WithDescription("Total thread count"))
	threadGauge.Record(ctx, int64(snap.ThreadCount), attrs)
}

// PublishSummary POSTs the summary and recommendations to the HTTP backend if
// configured. It is called on every heartbeat (Status="heartbeat") and on the
// final flush (Status="completed"), so the backend always has a record even if
// the agent is killed before the job finishes.
func (m *Manager) PublishSummary(ctx context.Context, summary types.MetricsSummary, recs []types.Recommendation) error {
	for _, b := range m.backends {
		switch b {
		case BackendHTTP:
			if err := m.publishHTTP(ctx, summary, recs); err != nil {
				return err
			}
		case BackendSlack:
			if err := m.postSlack(ctx, summary, recs); err != nil {
				return err
			}
		case BackendTeams:
			if err := m.postTeams(ctx, summary, recs); err != nil {
				return err
			}
		}
	}
	return nil
}

// publishHTTP POSTs the summary and recommendations to the HTTP backend.
func (m *Manager) publishHTTP(ctx context.Context, summary types.MetricsSummary, recs []types.Recommendation) error {
	if m.httpURL == "" {
		return fmt.Errorf("--http-url is required for the http exporter")
	}
	payload := struct {
		Summary         types.MetricsSummary   `json:"summary"`
		Recommendations []types.Recommendation `json:"recommendations"`
	}{summary, recs}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	apiKey := os.Getenv("RUNRIGHT_API_KEY")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.httpURL+"/api/v1/jobs", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("http backend returned %d", resp.StatusCode)
	}
	return nil
}

// Shutdown flushes and closes all OTEL providers.
func (m *Manager) Shutdown(ctx context.Context) {
	if m.provider != nil {
		_ = m.provider.Shutdown(ctx)
	}
	if m.promLn != nil {
		_ = m.promLn.Close()
	}
}

// postSlack sends a concise recommendation digest to a Slack incoming webhook.
// Only fires for completed summaries with at least one cheaper recommendation.
func (m *Manager) postSlack(ctx context.Context, summary types.MetricsSummary, recs []types.Recommendation) error {
	if m.slackWebhook == "" {
		return fmt.Errorf("--slack-webhook is required for the slack exporter")
	}
	if summary.Status != "completed" {
		return nil // only post on job completion, not heartbeats
	}

	// Find the best cheaper recommendation.
	var best *types.Recommendation
	for i := range recs {
		if recs[i].CostDeltaPercent < 0 {
			if best == nil || recs[i].CostDeltaPercent < best.CostDeltaPercent {
				best = &recs[i]
			}
		}
	}
	if best == nil {
		return nil // nothing to report — job is already right-sized
	}

	savingsPct := -best.CostDeltaPercent
	monthlyDelta := best.CurrentMonthly - best.EstimatedMonthly

	text := fmt.Sprintf(
		"RunRight savings report for `%s`\n"+
			">*Current:* `%s`   *Recommended:* `%s`\n"+
			">*Savings:* ~$%.2f/mo (%.0f%% cheaper)\n"+
			">*Required:* %d vCPU · %.1f GiB RAM (p95 with headroom)",
		summary.JobID,
		func() string {
			if summary.DetectedMachine != nil {
				return summary.DetectedMachine.ID
			}
			return "unknown"
		}(),
		best.Machine.ID,
		monthlyDelta,
		savingsPct,
		best.RequiredVCPUs,
		best.RequiredMemoryGiB,
	)

	payload := map[string]string{"text": text}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.slackWebhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack webhook returned %d", resp.StatusCode)
	}
	return nil
}

// postTeams sends a concise recommendation card to a Microsoft Teams Incoming Webhook.
// Only fires for completed summaries with at least one cheaper recommendation.
func (m *Manager) postTeams(ctx context.Context, summary types.MetricsSummary, recs []types.Recommendation) error {
	if m.teamsWebhook == "" {
		return fmt.Errorf("--teams-webhook is required for the teams exporter")
	}
	if summary.Status != "completed" {
		return nil
	}

	var best *types.Recommendation
	for i := range recs {
		if recs[i].CostDeltaPercent < 0 {
			if best == nil || recs[i].CostDeltaPercent < best.CostDeltaPercent {
				best = &recs[i]
			}
		}
	}
	if best == nil {
		return nil
	}

	currentID := "unknown"
	if summary.DetectedMachine != nil {
		currentID = summary.DetectedMachine.ID
	}
	savingsPct := -best.CostDeltaPercent
	monthlyDelta := best.CurrentMonthly - best.EstimatedMonthly

	// Teams Incoming Webhook payload (simple MessageCard format — universally supported).
	payload := map[string]any{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": "0078D4",
		"summary":    fmt.Sprintf("RunRight: %s can save ~$%.2f/mo", summary.JobID, monthlyDelta),
		"sections": []map[string]any{
			{
				"activityTitle":    fmt.Sprintf("RunRight - `%s`", summary.JobID),
				"activitySubtitle": fmt.Sprintf("Current: **%s** to Recommended: **%s**", currentID, best.Machine.ID),
				"facts": []map[string]string{
					{"name": "Monthly saving", "value": fmt.Sprintf("~$%.2f (%.0f%% cheaper)", monthlyDelta, savingsPct)},
					{"name": "Required", "value": fmt.Sprintf("%d vCPU · %.1f GiB RAM (p95 + headroom)", best.RequiredVCPUs, best.RequiredMemoryGiB)},
					{"name": "Spot risk", "value": best.SpotRisk},
				},
				"markdown": true,
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.teamsWebhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("teams webhook returned %d", resp.StatusCode)
	}
	return nil
}
