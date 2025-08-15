# Templates

`linear-cli` supports three template sources:
- Local files (search dirs)
- Remote base URL (http/https)
- Linear API (team-scoped)

## Resolution precedence
```mermaid
flowchart LR
  S[Start with --template / value] --> A{--template-id set?}
  A -- yes --> SID[Use API server-side template]
  A -- no --> B{--templates-source}
  B -- api --> API[Fetch via Linear API]
  B -- remote --> REM[Fetch via URL or base URL]
  B -- local --> LOC[Use local dirs]
  B -- auto --> C{Value form}
  C -- http(s):// --> REM
  C -- path-like --> LOC
  C -- name --> D{Try remote base}
  D -- hit --> REM
  D -- miss --> LOC
```

## Local search order
- `--templates-dir`
- `$LINEAR_TEMPLATES_DIR`
- `UserConfigDir/linear/templates`
- `~/.config/linear/templates`

## Remote base
- Flag: `--templates-base-url`
- Env: `LINEAR_TEMPLATES_BASE_URL`
- Names resolve to `<base>/<name>.md`
- Listing tries `<base>/index.json` containing `["bug", "feature"]` or `{ "templates": [...] }`

## Linear API
- List: team-scoped templates
- Preview: fetch by `id` or by `name` within team
- Create: server-side with `--template-id` if supported by `IssueCreateInput`

```mermaid
sequenceDiagram
  participant U as User
  participant CLI as linear-cli
  participant L as Local/Remote Store
  participant API as Linear GraphQL

  U->>CLI: issues template list --templates-source=api --team ENG
  CLI->>API: query team.issueTemplates
  API-->>CLI: [ {id,name,description}, ... ]
  CLI-->>U: Names

  U->>CLI: issues template preview feature --templates-source=api --team ENG
  CLI->>API: query issueTemplates(filter: {team,name})
  API-->>CLI: {id,name,description}
  CLI-->>U: Rendered markdown
```
