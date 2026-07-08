#!/usr/bin/env bash
# seed-demo.sh — Populates the RunRight database with realistic demo data
# Usage: ./scripts/seed-demo.sh [BASE_URL]
set -euo pipefail

BASE=${1:-http://localhost:8080}
API="$BASE/api/v1"

echo "=== Seeding RunRight Demo Data ==="
echo "Target: $API"
echo ""

# Helper function for API calls
api() {
  local method=$1 path=$2
  shift 2
  curl -sf -X "$method" "$API$path" \
    -H "Content-Type: application/json" \
    "$@" || true
}

# ─────────────────────────────────────────────────────────────────────────────
# 1. CREATE TEAMS
# ─────────────────────────────────────────────────────────────────────────────
echo "Creating teams..."

api POST /teams -d '{
  "name": "Platform Engineering",
  "slug": "platform-eng",
  "description": "Core infrastructure and DevOps tooling team"
}'

api POST /teams -d '{
  "name": "Data Science",
  "slug": "data-science", 
  "description": "Machine learning and analytics team"
}'

api POST /teams -d '{
  "name": "Frontend",
  "slug": "frontend",
  "description": "Web and mobile UI development"
}'

api POST /teams -d '{
  "name": "Backend Services",
  "slug": "backend",
  "description": "API and microservices team"
}'

api POST /teams -d '{
  "name": "QA & Testing",
  "slug": "qa",
  "description": "Quality assurance and test automation"
}'

api POST /teams -d '{
  "name": "Security",
  "slug": "security",
  "description": "Application security and compliance"
}'

echo "  ✓ Teams created"

# ─────────────────────────────────────────────────────────────────────────────
# 2. CREATE POLICIES
# ─────────────────────────────────────────────────────────────────────────────
echo "Creating policies..."

# Global default policy
api PUT /policies -d '{
  "repository": "",
  "job_id": "",
  "max_cost_per_hour": 1.00,
  "enabled": true
}'

# Per-repo policies
api PUT /policies -d '{
  "repository": "runrightio/app-core",
  "job_id": "",
  "max_cost_per_hour": 0.50,
  "enabled": true
}'

api PUT /policies -d '{
  "repository": "runrightio/web-ui",
  "job_id": "",
  "max_cost_per_hour": 0.75,
  "enabled": true
}'

api PUT /policies -d '{
  "repository": "runrightio/data-pipeline",
  "job_id": "",
  "max_cost_per_hour": 5.00,
  "enabled": true
}'

api PUT /policies -d '{
  "repository": "runrightio/ml-platform",
  "job_id": "",
  "max_cost_per_hour": 10.00,
  "enabled": true
}'

api PUT /policies -d '{
  "repository": "runrightio/infrastructure",
  "job_id": "",
  "max_cost_per_hour": 2.00,
  "enabled": true
}'

# Job-specific policies
api PUT /policies -d '{
  "repository": "runrightio/data-pipeline",
  "job_id": "gpu-inference",
  "max_cost_per_hour": 35.00,
  "enabled": true
}'

api PUT /policies -d '{
  "repository": "runrightio/ml-platform",
  "job_id": "train-model",
  "max_cost_per_hour": 40.00,
  "enabled": true
}'

api PUT /policies -d '{
  "repository": "runrightio/app-core",
  "job_id": "build",
  "max_cost_per_hour": 0.25,
  "enabled": true
}'

echo "  ✓ Policies created"

# ─────────────────────────────────────────────────────────────────────────────
# 3. CREATE OWNERSHIP MAPPINGS
# ─────────────────────────────────────────────────────────────────────────────
echo "Creating ownership mappings..."

api PUT /ownership -d '{
  "repository": "runrightio/app-core",
  "team_name": "Backend Services"
}'

api PUT /ownership -d '{
  "repository": "runrightio/web-ui",
  "team_name": "Frontend"
}'

api PUT /ownership -d '{
  "repository": "runrightio/data-pipeline",
  "team_name": "Data Science"
}'

api PUT /ownership -d '{
  "repository": "runrightio/ml-platform",
  "team_name": "Data Science"
}'

api PUT /ownership -d '{
  "repository": "runrightio/infrastructure",
  "team_name": "Platform Engineering"
}'

api PUT /ownership -d '{
  "repository": "runrightio/security-scanner",
  "team_name": "Security"
}'

api PUT /ownership -d '{
  "repository": "runrightio/test-automation",
  "team_name": "QA & Testing"
}'

echo "  ✓ Ownership mappings created"

# ─────────────────────────────────────────────────────────────────────────────
# 3.5 SEED POOL CONSTRAINTS (Machine Selection Policies)
# ─────────────────────────────────────────────────────────────────────────────
echo "Creating pool constraints..."

api PUT /user-settings -d '{
  "otel_endpoint": "",
  "allowed_machine_ids": [],
  "allowed_series": ["t3", "m6i", "c7g", "m7g", "r7g"],
  "allowed_families": ["general-purpose", "compute-optimized"]
}'

echo "  ✓ Pool constraints created"

# ─────────────────────────────────────────────────────────────────────────────
# 4. SEED NOTIFICATION SETTINGS & ALERTS
# ─────────────────────────────────────────────────────────────────────────────
echo "Creating notification settings..."

api PUT /notifications/settings -d '{
  "enabled": true,
  "events": {
    "policy_violation": true,
    "high_waste": true,
    "daily_summary": true
  },
  "slack": {
    "enabled": true,
    "webhook_url": "",
    "channel": "",
    "mention": "",
    "destinations": [
      {
        "id": "slack-1",
        "name": "#ci-costs",
        "webhook_url": "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXX",
        "channel": "#ci-costs",
        "mention": ""
      },
      {
        "id": "slack-2",
        "name": "#ml-alerts",
        "webhook_url": "https://hooks.slack.com/services/T00000000/B11111111/YYYYYYYYYYYYYYYYYYYY",
        "channel": "#ml-alerts",
        "mention": "@ml-team"
      }
    ]
  },
  "teams": {
    "enabled": false,
    "destinations": []
  },
  "webhooks": {
    "enabled": true,
    "destinations": [
      {
        "id": "webhook-1",
        "name": "PagerDuty",
        "url": "https://events.pagerduty.com/v2/enqueue"
      }
    ]
  },
  "rules": [
    {
      "id": "rule-1",
      "name": "High Cost Jobs",
      "type": "threshold",
      "scope": "global",
      "repository": "",
      "jobId": "",
      "metric": "max_cost_per_hour",
      "threshold": 1.0,
      "destinationIds": ["slack-1"],
      "enabled": true
    },
    {
      "id": "rule-2",
      "name": "GPU Job Alerts",
      "type": "threshold",
      "scope": "repository",
      "repository": "runrightio/data-pipeline",
      "jobId": "",
      "metric": "max_cost_per_hour",
      "threshold": 5.0,
      "destinationIds": ["slack-2"],
      "enabled": true
    },
    {
      "id": "rule-3",
      "name": "Policy Violations",
      "type": "event",
      "event": "policy_violation",
      "scope": "global",
      "repository": "",
      "jobId": "",
      "metric": "max_cost_per_hour",
      "threshold": 0,
      "destinationIds": ["webhook-1"],
      "enabled": true
    },
    {
      "id": "rule-4",
      "name": "Daily Digest",
      "type": "event",
      "event": "daily_summary",
      "scope": "global",
      "repository": "",
      "jobId": "",
      "metric": "max_cost_per_hour",
      "threshold": 0,
      "destinationIds": ["slack-1"],
      "enabled": true
    },
    {
      "id": "rule-5",
      "name": "High Waste Alert",
      "type": "event",
      "event": "high_waste",
      "scope": "global",
      "repository": "",
      "jobId": "",
      "metric": "max_cost_per_hour",
      "threshold": 0,
      "destinationIds": ["slack-1", "webhook-1"],
      "enabled": true
    }
  ],
  "email": {
    "enabled": false,
    "recipients": [],
    "subject_prefix": "[RunRight]"
  }
}'

echo "  ✓ Notification settings created"

# Seed alert delivery logs directly via PostgreSQL
echo "Seeding alert delivery history..."

# Get postgres connection from docker-compose
PGHOST="${PGHOST:-localhost}"
PGPORT="${PGPORT:-5435}"
PGUSER="${PGUSER:-runright}"
PGPASSWORD="${PGPASSWORD:-runright}"
PGDATABASE="${PGDATABASE:-runright}"

# Insert sample notification delivery logs
docker compose exec -T postgres psql -U "$PGUSER" -d "$PGDATABASE" <<'EOSQL'
INSERT INTO notification_delivery_logs (rule_id, destination_id, channel, job_id, repository, status, error_message, sent_at)
VALUES
  ('rule-1', 'slack-1', 'slack', 'gpu-inference', 'runrightio/data-pipeline', 'delivered', '', NOW() - INTERVAL '1 day'),
  ('rule-1', 'slack-1', 'slack', 'train-model', 'runrightio/ml-platform', 'delivered', '', NOW() - INTERVAL '1 day'),
  ('rule-2', 'slack-2', 'slack', 'gpu-inference', 'runrightio/data-pipeline', 'delivered', '', NOW() - INTERVAL '2 days'),
  ('rule-2', 'slack-2', 'slack', 'gpu-inference', 'runrightio/data-pipeline', 'delivered', '', NOW() - INTERVAL '3 days'),
  ('rule-1', 'slack-1', 'slack', 'ml-batch', 'runrightio/data-pipeline', 'delivered', '', NOW() - INTERVAL '4 days'),
  ('rule-3', 'webhook-1', 'webhook', 'build', 'runrightio/app-core', 'failed', 'Connection refused', NOW() - INTERVAL '5 days'),
  ('rule-1', 'slack-1', 'slack', 'data-transform', 'runrightio/data-pipeline', 'delivered', '', NOW() - INTERVAL '5 days'),
  ('rule-2', 'slack-2', 'slack', 'gpu-inference', 'runrightio/data-pipeline', 'delivered', '', NOW() - INTERVAL '6 days'),
  ('rule-1', 'slack-1', 'slack', 'model-eval', 'runrightio/ml-platform', 'delivered', '', NOW() - INTERVAL '7 days'),
  ('rule-3', 'webhook-1', 'webhook', 'e2e-tests', 'runrightio/web-ui', 'delivered', '', NOW() - INTERVAL '8 days'),
  ('rule-1', 'slack-1', 'slack', 'load-tests', 'runrightio/test-automation', 'delivered', '', NOW() - INTERVAL '9 days'),
  ('rule-2', 'slack-2', 'slack', 'gpu-inference', 'runrightio/data-pipeline', 'delivered', '', NOW() - INTERVAL '10 days'),
  ('rule-1', 'slack-1', 'slack', 'train-model', 'runrightio/ml-platform', 'delivered', '', NOW() - INTERVAL '12 days'),
  ('rule-3', 'webhook-1', 'webhook', 'benchmark', 'runrightio/data-pipeline', 'failed', 'Timeout', NOW() - INTERVAL '14 days'),
  ('rule-1', 'slack-1', 'slack', 'ml-batch', 'runrightio/data-pipeline', 'delivered', '', NOW() - INTERVAL '15 days'),
  ('rule-2', 'slack-2', 'slack', 'gpu-inference', 'runrightio/data-pipeline', 'delivered', '', NOW() - INTERVAL '18 days'),
  ('rule-1', 'slack-1', 'slack', 'data-transform', 'runrightio/data-pipeline', 'delivered', '', NOW() - INTERVAL '20 days'),
  ('rule-1', 'slack-1', 'slack', 'ios-build', 'runrightio/mobile-app', 'delivered', '', NOW() - INTERVAL '22 days'),
  ('rule-3', 'webhook-1', 'webhook', 'dast-scan', 'runrightio/security-scanner', 'delivered', '', NOW() - INTERVAL '25 days'),
  ('rule-1', 'slack-1', 'slack', 'gpu-inference', 'runrightio/data-pipeline', 'delivered', '', NOW() - INTERVAL '28 days')
ON CONFLICT DO NOTHING;
EOSQL

echo "  ✓ Alert history seeded"

# ─────────────────────────────────────────────────────────────────────────────
# 5. SEED JOBS (via existing Go seed tool for detailed job data)
# ─────────────────────────────────────────────────────────────────────────────
echo "Seeding jobs..."
cd "$(dirname "$0")/.."
go run ./cmd/seed --url "$BASE"

echo ""
echo "=== Demo data seeding complete ==="
