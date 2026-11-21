---
paths:
  - "pkg/**"
---

# Packages built to move to world-engine

- **`pkg/level`, `pkg/level/bake`, `pkg/levelstate` and `pkg/queue` are built to move to world-engine,**
  so they follow world-engine's rules, not this repo's: `level` imports only `box2d` and the standard
  library, `bake` adds `level`, `levelstate` adds `cardinal` and `level`, `queue` imports only
  `cardinal`, `queue/queuetest` adds `queue` and OpenTelemetry's test recorder, and none names a game word; JSON is `goccy/go-json`;
  tests are external packages (`level_test`). Lint them with world-engine's config and its pinned
  linter (v2.15.0, built with this repo's Go): `golangci-lint run --config
  ../world-engine/.golangci.yaml --fix=false ./pkg/level/... ./pkg/levelstate/... ./pkg/queue/...`.
