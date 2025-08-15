# Changelog

All notable changes to this project will be documented in this file.

The format follows Keep a Changelog, and this project adheres to Semantic Versioning.

## [v0.1.2] - 2025-08-15
### Added
- Shell completion command: `linear-cli completion [bash|zsh|fish|powershell]`

### Changed
- Richer, more informative help output with usage, examples, env/config hints
- Global flags: `--output|-o json|text` (alias of `--json`)
- Help/version shorthands: `-h` and `-v`; also support `linear-cli help` and `linear-cli version`

## [v0.1.1] - 2025-08-14
### Changed
- Add `--version`; embed version/commit in release binaries
- Add CHANGELOG and clarify installation docs (Homebrew tap, release binaries)
- Improve release workflow: permissions, manual dispatch, auto-generate notes from CHANGELOG

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

[Unreleased]: https://github.com/nikpietanze/linear-cli/compare/v0.1.2...HEAD
[v0.1.2]: https://github.com/nikpietanze/linear-cli/releases/tag/v0.1.2
[v0.1.1]: https://github.com/nikpietanze/linear-cli/releases/tag/v0.1.1
[v0.1.0]: https://github.com/nikpietanze/linear-cli/releases/tag/v0.1.0
