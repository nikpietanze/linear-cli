# Changelog

All notable changes to this project will be documented in this file.

The format follows Keep a Changelog, and this project adheres to Semantic Versioning.

## [v0.1.0] - 2025-08-14
### Added
- Initial public release of `linear-cli`.
- Auth: `login`, `status`, and `test` commands.
- Projects: `projects list`.
- Issues: `issues list` with filters (`--project`, `--assignee`, `--state`, quick flags `--todo/--doing/--done`), `issues view <id|TEAM-123>`, `issues create` with `--project/--assignee/--label/--priority`.
- Comments: `comment create`.
- Output: `--json` output mode and tabular output by default.
- Safety: mutation allowlist and guard against delete/archive.
- Tests: internal API tests and command-level test for `issues view`.
- CI: GitHub Actions release workflow publishing darwin/linux for amd64 and arm64.

[Unreleased]: https://github.com/nikpietanze/linear-cli/compare/v0.1.0...HEAD
[v0.1.0]: https://github.com/nikpietanze/linear-cli/releases/tag/v0.1.0
