package server

import (
	"net/http"
	"testing"
)

func validNotificationPayload() map[string]any {
	return map[string]any{
		"enabled": true,
		"events": map[string]any{
			"policy_violation": true,
			"high_waste":       false,
			"daily_summary":    true,
		},
		"slack": map[string]any{
			"enabled": true,
			"destinations": []map[string]any{
				{
					"id":          "dest-primary",
					"name":        "Primary",
					"webhook_url": "https://hooks.slack.com/services/T000/B000/XXXX",
					"channel":     "#alerts",
					"mention":     "",
				},
			},
		},
		"rules": []map[string]any{
			{
				"id":             "rule-threshold",
				"name":           "High cost",
				"type":           "threshold",
				"scope":          "global",
				"repository":     "",
				"jobId":          "",
				"metric":         "max_cost_per_hour",
				"threshold":      0.5,
				"destinationIds": []string{"dest-primary"},
				"enabled":        true,
			},
		},
		"email": map[string]any{
			"enabled":        false,
			"recipients":     []string{},
			"subject_prefix": "[RunRight]",
		},
	}
}

func resetNotificationState(t *testing.T, s *Server) {
	t.Helper()
	if _, err := s.db.Exec(`DELETE FROM notification_settings`); err != nil {
		t.Fatalf("reset notification_settings: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM notification_destination_secrets`); err != nil {
		t.Fatalf("reset notification_destination_secrets: %v", err)
	}
	if _, err := s.db.Exec(`DELETE FROM notification_daily_summary_dispatches`); err != nil {
		t.Fatalf("reset notification_daily_summary_dispatches: %v", err)
	}
}

func TestNotificationRuleValidationAcceptsValidPayload(t *testing.T) {
	s := newPolicyTestServer(t)
	resetNotificationState(t, s)

	code := doJSON(t, s, http.MethodPut, "/api/v1/notifications/settings", validNotificationPayload(), nil)
	if code != http.StatusOK {
		t.Fatalf("expected 200 for valid payload, got %d", code)
	}
}

func TestNotificationRuleValidationRejectsMalformedRules(t *testing.T) {
	s := newPolicyTestServer(t)

	tests := []struct {
		name   string
		mutate func(payload map[string]any)
	}{
		{
			name: "unknown destination reference",
			mutate: func(payload map[string]any) {
				rules := payload["rules"].([]map[string]any)
				rules[0]["destinationIds"] = []string{"does-not-exist"}
			},
		},
		{
			name: "repository scope without repository",
			mutate: func(payload map[string]any) {
				rules := payload["rules"].([]map[string]any)
				rules[0]["scope"] = "repository"
			},
		},
		{
			name: "job scope without job id",
			mutate: func(payload map[string]any) {
				rules := payload["rules"].([]map[string]any)
				rules[0]["scope"] = "job"
				rules[0]["repository"] = "owner/repo"
				rules[0]["jobId"] = ""
			},
		},
		{
			name: "threshold rule with unsupported metric",
			mutate: func(payload map[string]any) {
				rules := payload["rules"].([]map[string]any)
				rules[0]["metric"] = "unknown_metric"
			},
		},
		{
			name: "threshold rule with non-positive threshold",
			mutate: func(payload map[string]any) {
				rules := payload["rules"].([]map[string]any)
				rules[0]["threshold"] = 0
			},
		},
		{
			name: "event rule with unsupported event",
			mutate: func(payload map[string]any) {
				rules := []map[string]any{
					{
						"id":             "rule-event",
						"name":           "Bad event",
						"type":           "event",
						"event":          "not_real",
						"scope":          "global",
						"repository":     "",
						"jobId":          "",
						"metric":         "max_cost_per_hour",
						"threshold":      0,
						"destinationIds": []string{"dest-primary"},
						"enabled":        true,
					},
				}
				payload["rules"] = rules
			},
		},
		{
			name: "event rule with non-zero threshold",
			mutate: func(payload map[string]any) {
				rules := []map[string]any{
					{
						"id":             "rule-event",
						"name":           "Bad threshold",
						"type":           "event",
						"event":          "policy_violation",
						"scope":          "global",
						"repository":     "",
						"jobId":          "",
						"metric":         "max_cost_per_hour",
						"threshold":      0.1,
						"destinationIds": []string{"dest-primary"},
						"enabled":        true,
					},
				}
				payload["rules"] = rules
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetNotificationState(t, s)
			payload := validNotificationPayload()
			tc.mutate(payload)
			code := doJSON(t, s, http.MethodPut, "/api/v1/notifications/settings", payload, nil)
			if code != http.StatusBadRequest {
				t.Fatalf("expected 400 for malformed rule payload, got %d", code)
			}
		})
	}
}
