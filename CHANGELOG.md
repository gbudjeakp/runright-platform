# Changelog

All notable changes to RunRight will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.5.0] - 2026-07-04

### Added

- **Email Notifications**: New email destination channel for alerts alongside Slack, Teams, and webhooks
  - Configure SMTP settings via environment variables (`RUNRIGHT_SMTP_HOST`, `RUNRIGHT_SMTP_USER`, `RUNRIGHT_SMTP_PASS`, `RUNRIGHT_SMTP_FROM`)
  - Customizable subject prefix per organization
  - Multiple email recipients supported
  - Works in demo mode without SMTP (logs to console)

- **Analytics Dashboard Improvements**:
  - Cost breakdown by repository, job, and runner type
  - Trend analysis with 7d, 30d, and 90d periods
  - Waste percentage visualization
  - Monthly savings projections

- **Alerts Page UX Enhancements**:
  - Search bar for filtering Alert Rules
  - Search bar for filtering Ownership mappings
  - Ownership form moved to top of tab for better discoverability
  - Email tab in Destinations with subject prefix and recipients management

- **Demo Seeding Improvements**:
  - Balanced cost distribution across repositories
  - Reduced GPU job frequency for more realistic data
  - Pool constraints with allowed instance series/families
  - Notification settings and delivery logs in seed data

### Changed

- Alert validation now accepts email as a valid destination channel
- Notification dispatcher includes email destinations when enabled
- Default email subject prefix set to `[RunRight]`

### Fixed

- Ownership entries now properly filter by search query
- Rules list shows "No rules match" empty state when filtered

## [1.4.0] - 2026-06-15

### Added

- Policy guardrails with max cost/hour limits
- Threshold-based alert rules (cost, waste percentage)
- Event-based alert rules (policy violation, high waste, daily summary)
- Multi-destination alert routing
- Ownership-based alert routing by team
- Delivery logs with retry scheduling

## [1.3.0] - 2026-05-20

### Added

- Microsoft Teams webhook support
- Custom webhook destinations for external integrations
- Alert rule enable/disable toggle
- Repository-scoped and job-scoped rules

## [1.2.0] - 2026-04-10

### Added

- Multi-destination Slack webhooks
- Alert rules management UI
- Daily summary notifications at 09:00 UTC

## [1.1.0] - 2026-03-01

### Added

- GCP instance catalog alongside AWS
- Currency conversion support
- Prometheus metrics export

## [1.0.0] - 2026-01-15

### Added

- Initial release
- CPU, memory, disk I/O, and thread profiling
- AWS instance recommendations
- Self-hosted dashboard and API
- GitHub Actions integration
- OTLP, Prometheus, and JSON export modes
