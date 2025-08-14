# Security policy for linear-cli

- No destructive commands are implemented. There is no delete/archive functionality.
- Transport guard: All GraphQL requests are validated. Any mutation containing words like "delete" or "archive" is rejected. Additionally, only a small allowlist of mutations is permitted (currently: `issueCreate`).
- Credentials are stored locally in `~/.config/linear/config.toml` with file permissions `0600`. Environment variable `LINEAR_API_KEY` overrides the file.
- On HTTP 429/5xx responses, the client retries with exponential backoff and honors `Retry-After` when provided.

If you discover a security issue, please open a GitHub issue or contact the maintainers.
