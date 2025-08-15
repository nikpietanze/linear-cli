# Architecture Overview

## Components
- CLI (Cobra commands) under `cmd/`
- Linear GraphQL client under `internal/api/`
- Config loader under `internal/config/`
- Output utilities under `internal/output/`

## Issue creation paths
```mermaid
flowchart TD
  subgraph CLI
    C1[issues create]
    C2[Template resolver]
    C3[fillTemplate]
  end
  subgraph Sources
    L[Local]
    R[Remote HTTP]
    A[Linear API]
  end
  subgraph Linear
    GQL[GraphQL API]
  end

  C1 --> C2
  C2 -->|local| L
  C2 -->|remote| R
  C2 -->|api| A
  L --> C3
  R --> C3
  A --> C3
  C3 --> GQL
```

## Files of interest
- `cmd/issues_adv.go`: flags, template resolution, interactive prompts
- `internal/api/linear.go`: GraphQL queries/mutations, template helpers
