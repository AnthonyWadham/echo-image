---
paths:
  - "client/**"
  - "shards/game/predictor/**"
  - "go.mod"
  - "sim.go"
---

# Client build and toolchain

- **The client needs Go too.** `bun run dev` and `bun run build` (in `client/`) first build
  `predictor.wasm` and copy that toolchain's `wasm_exec.js` (`client/scripts/predictor.mjs`, both
  gitignored). Vitest runs the same script as its global setup, so its tests never use a stale build.
  It stamps the wasm with the tree's sim version (see client sync), so after a Go or content change
  that reaches the shard, restart `bun run dev` too.
- **Toolchain:** go.mod's go line (1.27.4) is the Go the shard's Docker build runs (the world CLI's
  golang image), and `SimVersion` hashes the release. `predictor.mjs` builds with exactly that Go
  (`GOTOOLCHAIN`), whatever is installed. Bump it with the CLI's image.
- **Client pairing.** The Unity client (`/mnt/z/Echo`) still uses `dev.world.sdk` v0.6.0 and its
  generated protobuf C#, which doesn't match the MessagePack payloads. It needs updating before it
  can talk to this shard again.
