<p align="center">
  <img src="web/public/favicon.svg" width="72" height="72" alt="RunRight logo" />
</p>

<h1 align="center">RunRight Platform</h1>

<p align="center"><strong>Self-hosted dashboard and API for <a href="https://github.com/gbudjeakp/run-right">RunRight</a>.</strong></p>

<p align="center">Web UI · REST API · PostgreSQL · AI cost assistant · SSO · RBAC · Grafana</p>

---

<p align="center"><sub>Live product tour: dashboard, run insights, recommendations, and job history</sub></p>

![RunRight UI carousel: jobs, run detail, recommendations, and history](.github/screenshots/readme-carousel.gif)

<p align="center"><sub>Pipe your data into Grafana or use the built-in dashboard. Raw runs land in Postgres.</sub></p>
<p align="center">
  <img src=".github/screenshots/ss-06.png" width="98%" alt="Grafana dashboard: CI compute sizes" />
</p>

---

## What this is

The RunRight agent ([run-right](https://github.com/gbudjeakp/run-right)) collects metrics from every CI job. **RunRight Platform** is the optional self-hosted backend that stores those runs, serves a web dashboard, and powers the AI cost assistant.

You do **not** need this repo to use RunRight. The agent works standalone with OTLP, Prometheus, or JSON file export. This repo is for teams that want a persistent dashboard, historical analysis, and org-level reporting.

---

## Quick start

```bash
export RUNRIGHT_API_KEY=$(openssl rand -hex 32)
docker compose up -d
```

| Service   | URL                    |
|-----------|------------------------|
| Dashboard | http://localhost:3000  |
| API       | http://localhost:8080  |
| PostgreSQL | localhost:5435        |

Point your agents at the platform by adding to your CI:

```yaml
- uses: gbudjeakp/run-right@v1
  with:
    run: make build
  env:
    RUNRIGHT_URL: ${{ vars.RUNRIGHT_URL }}
    RUNRIGHT_API_KEY: ${{ secrets.RUNRIGHT_API_KEY }}
```

---

## Features

| Feature | Details |
|---------|---------|
| **Jobs dashboard** | Browse every CI run: duration, CPU p95, memory, tier label, cost delta |
| **Run detail** | Per-run timeline, resource breakdown, and instance recommendation |
| **AI cost assistant** | Chat interface backed by OpenAI, Anthropic, or a local Ollama model |
| **Policies** | Org-level rules that auto-flag jobs exceeding cost or resource thresholds |
| **Alerts** | Slack, webhook, or email notifications when policy rules trigger |
| **SSO** | Google, GitHub OAuth2, or SAML 2.0 |
| **RBAC** | Viewer / Member / Admin / Owner roles per team |
| **Audit log** | Immutable record of config changes |
| **Grafana** | Pre-built dashboard auto-provisioned from `grafana/` |

---

## Configuration

Copy `.env.example` to `.env` and fill in your values:

```bash
cp .env.example .env
```

Key variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `DATABASE_URL` | `postgres://...@localhost:5435/runright` | PostgreSQL connection string |
| `RUNRIGHT_API_KEY` | — | Shared secret for agent → API auth |
| `RUNRIGHT_BASE_URL` | `http://localhost:8080` | Public URL (used for OAuth callbacks) |
| `RUNRIGHT_AI_PROVIDER` | `ollama` | `ollama` \| `openai` \| `anthropic` |
| `RUNRIGHT_AI_MODEL` | `llama3.1` | Model name for the AI assistant |
| `RUNRIGHT_SSO_ENABLED` | `false` | Enable SSO login |

See `.env.example` for the full list.

---

## Dev

**Prerequisites:** Go 1.22+, Node.js 20+, pnpm 9+, Docker (for Postgres)

```bash
# Start Postgres only
docker compose up -d postgres

# Backend (hot reload via Air)
air

# Frontend
cd web && pnpm install && pnpm dev
```

Or run backend + frontend without Docker:

```bash
# Terminal 1
DATABASE_URL="postgres://runright:runright@localhost:5435/runright?sslmode=disable" \
  go run ./cmd/server --port 8080

# Terminal 2
cd web && pnpm dev
```

### Docker dev profile (backend hot reload)

```bash
docker compose up -d postgres
docker compose --profile dev up -d backend-dev
# Edits to *.go reload automatically; no restart needed
```

---

## Grafana

A pre-built dashboard is included in `grafana/`. It auto-provisions when you run Docker Compose:

```bash
docker compose up -d
# Grafana at http://localhost:3001  (admin / runright)
```

Panels: jobs/day · avg CPU p95 by job · CI platform breakdown · cost savings potential · recent runs table · CPU trend by CI platform.

---

## Architecture

```
web/          React 18 + Vite + Tailwind CSS — dashboard
cmd/server/   Go entrypoint (Gin, port 8080)
cmd/seed/     Demo data seeder
internal/
  server/     REST API handlers, auth middleware, RBAC, SSO, audit
  assistant/  AI cost assistant (OpenAI / Anthropic / Ollama)
  embeddings/ RAG vector search over job history
  engine/     Cost recommender (shared with agent)
  catalog/    AWS + GCP instance catalog
  currency/   Exchange rate helpers
  notification/ Alert dispatch (Slack, webhook, email)
helm/         Kubernetes Helm chart
terraform/    EKS + GKE infrastructure
grafana/      Dashboards and provisioning config
```

---

## Related

- **[run-right](https://github.com/gbudjeakp/run-right)** — the agent and GitHub Action (no server required)

---

Elastic License 2.0 (ELv2). See [LICENSE](LICENSE)
