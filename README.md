# linear-cli

A fast, portable CLI to authenticate with Linear and read/create issues via Linear's GraphQL API. Designed for local use and CI/CD.

## Features

- Auth: login, status, and connectivity test
- Projects: list projects (name, id, status)
- Issues: list with filters, view full details, and create
- JSON output for scripting, tabular by default
- No-delete safety: destructive operations are not implemented and blocked at transport

## Requirements

- Go 1.22 or newer
- A Linear API key (create one in Linear: Settings → API)

## Installation

Option 1: Build and install from source

```bash
# Clone the repo
git clone https://github.com/your-org/linear-cli.git
cd linear-cli

# Build and install the binary into $GOBIN (or $GOPATH/bin)
go install .

# Ensure your Go bin is on PATH (add to your shell config if needed)
export PATH="$(go env GOPATH)/bin:$PATH"

# Verify
linear-cli --help
```

Option 2: Local build without installing globally

```bash
# From the project root
go build -o ./dist/linear-cli .
./dist/linear-cli --help
```

## Authentication

You can pass your API key via flag, environment, or interactive prompt.

- Flag: `linear-cli auth login --token <YOUR_TOKEN>`
- Environment: `export LINEAR_API_KEY=<YOUR_TOKEN>` then `linear-cli auth login`
- Prompt: run `linear-cli auth login` and paste when prompted

Check status:

```bash
linear-cli auth status

CI health check (non-zero exit on failure):

```bash
linear-cli auth test
```
```

## Usage

List recent issues (optionally by team key):

```bash
linear-cli issues list --limit 10 --project "Website" --assignee "Jane" --state "In Progress"
```

Get an issue by ID or key:

```bash
linear-cli issues get --id <ISSUE_ID>
linear-cli issues get --key ENG-123

View full details:

```bash
linear-cli issues view <ISSUE_ID>
```
```

Create an issue:

```bash
linear-cli issues create --title "New bug" --description "Steps to reproduce..." --project "Website" --assignee "Jane" --label "bug" --priority 2

List projects:

```bash
linear-cli projects list
```
```

## Configuration

- Env var: `LINEAR_API_KEY`
- Config file (optional): `~/.config/linear/config.toml`

Example TOML:

```toml
api_key = "lin_xxx..."
```

The config file stores only your API key (permissions `0600`). Environment variable overrides file values.

## Uninstall

Remove the binary from your Go bin and delete the config directory:

```bash
rm -f "$(go env GOPATH)/bin/linear-cli"
rm -rf ~/.config/linear
```

## Security

This CLI intentionally does not implement delete/archive operations. At the transport layer, a guard rejects any GraphQL mutation that attempts deletion or archival, and only a small allowlist of mutations is permitted (currently: issueCreate).

## License

MIT License. See [LICENSE](./LICENSE).
