# Configuration

## Authentication
- API key stored in `~/.config/linear/config.toml` under `api_key`
- Env override: `LINEAR_API_KEY`

## Template sources
- Local dir override: `--templates-dir`, env `LINEAR_TEMPLATES_DIR`
- Remote base: `--templates-base-url`, env `LINEAR_TEMPLATES_BASE_URL`
- Source selector: `--templates-source` = `auto|local|remote|api`
- Server-side creation: `--template-id` (requires `--team`)

## Behavior flags
- `--interactive` / `--no-interactive`
- `--preview` / `--no-preview` / `--yes`
- `--fail-on-missing`
- `--var KEY=VALUE` (repeatable) and `--vars-file file.json`

## Notable defaults
- Interactive defaults to on when using `--template` without `--description`.
- Preview defaults to on when any prefill vars are provided (disable with `--no-preview`).
