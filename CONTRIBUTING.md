# Contributing to RunRight

Thanks for your interest. Here's what you need to know before opening a PR or issue.

## Ground rules

- **AI-assisted contributions are welcome.** Using Copilot, Cursor, Claude, or similar tools to help write code or tests is fine — just make sure you've read, understood, and tested what you're submitting.
- **No automated bots.** Do not use tools that programmatically open issues or pull requests without direct human review and intent. Mass-generated issues, duplicate reports, and bulk PRs will be closed without response.
- **One concern per PR.** Keep pull requests focused. A PR that fixes a bug and adds a feature will be asked to split.

## Opening an issue

Before opening, search existing issues. If yours is new:

- **Bug**: describe what you did, what you expected, what happened. Include the runner OS, Go version, and the relevant `runright` output.
- **Feature request**: explain the use case, not just the implementation. "I want flag X" is less useful than "I'm running 500 CI jobs/day and can't distinguish them because…"

## Pull requests

1. Fork the repo and create a branch from `main`.
2. Run `go test ./...` and `go vet ./...` before opening.
3. If you changed the binary, update `README.md` usage examples if relevant.
4. Keep commit messages in the conventional format: `fix:`, `feat:`, `chore:`, `docs:`.

## What we won't merge

- PRs that change the license or remove attribution
- Dependencies with GPL or AGPL licenses (incompatible with ELv2)
- Anything that phones home, collects telemetry, or exfiltrates data
