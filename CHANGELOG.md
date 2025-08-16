# Changelog

All notable changes to this project will be documented in this file.

The format follows Keep a Changelog, and this project adheres to Semantic Versioning.

## [v0.2.0] - 2025-01-27
### Added
- **🤖 AI-Optimized Issue Creation**: Single-command issue creation designed for AI agents and automation
  - `--template` and `--sections` flags for structured input: `linear-cli issues create --team ENG --template "Feature Template" --title "Add search" --sections Summary="..." --sections Context="..."`
  - Auto-discovery: automatically syncs templates when not cached locally
  - Intelligent fallbacks: seamless operation without manual setup
- **📋 Hybrid Template System**: Fast local caching with server-side consistency
  - `templates sync` command with intelligent sync (detects new/updated/removed templates)
  - Local template storage in `~/Library/Application Support/linear/templates/`
  - Reference issues (`[TEMPLATE-REF]` prefix) for permanent template content extraction
  - `templates list`, `templates status`, and `templates clean` commands
- **🔍 Template Structure Discovery**: `issues template structure` command for AI agents to discover template sections dynamically
- **📝 Dynamic Section Filling**: Template sections parsed and filled dynamically from actual Linear templates (Spike, Bug, Feature, etc.)
- **🚀 Enhanced Documentation**: 
  - AI-agent focused README with clear workflow examples
  - New `docs/ai-examples.md` with GitHub Actions, Python/JS, Slack bot integration examples
  - Example GitHub Actions workflow file
- **⚡ Performance Improvements**: Local template caching eliminates API calls during issue creation

### Changed
- **Breaking**: Repositioned CLI as "AI-optimized CLI for Linear issue management"
- Interactive issue creation now uses server-side template application for consistency
- Help text and examples updated to emphasize AI-agent workflows
- Issue creation defaults to sensible priority (Medium) and state (Todo/Backlog)

### Fixed
- Template description population now works correctly with Linear's server-side template application
- Multiple template sections now fill properly (fixed bug where only first section was processed)
- Removed binary from git tracking and added to .gitignore

### Security
- Enforced "no delete" policy: CLI cannot delete Linear issues or projects
- Template sync uses permanent reference issues instead of temporary ones

## [v0.1.2] - 2025-08-15
## [v0.1.3] - 2025-08-15
### Fixed
- GraphQL variable types aligned with Linear schema to resolve 400 errors when viewing issues:
  - Use `String!` for `issue(id: ...)`
  - Use `ID!` for `team.id` filters
- Added regression tests to enforce correct variable types

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

[Unreleased]: https://github.com/nikpietanze/linear-cli/compare/v0.2.0...HEAD
[v0.2.0]: https://github.com/nikpietanze/linear-cli/releases/tag/v0.2.0
[v0.1.3]: https://github.com/nikpietanze/linear-cli/releases/tag/v0.1.3
[v0.1.2]: https://github.com/nikpietanze/linear-cli/releases/tag/v0.1.2
[v0.1.1]: https://github.com/nikpietanze/linear-cli/releases/tag/v0.1.1
[v0.1.0]: https://github.com/nikpietanze/linear-cli/releases/tag/v0.1.0
