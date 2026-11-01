// Builds the client predictor (shards/game/predictor) to wasm with the Go toolchain the server is built
// with: go.mod's go line, which the shard's Docker build runs, whatever Go is installed here. It copies
// that toolchain's wasm_exec.js next to its loader, so the two always match. The dev
// and build scripts run it first, and vitest runs it as its global setup, which also has the Go test
// write its prediction run for the wasm to match (predictor.test.ts). It stamps the wasm with this
// tree's sim version, which a shard built from it sends in body_sync. GOFLAGS=-tags=rate60 builds all
// three at 60 Hz.
import { execFileSync } from 'node:child_process'
import { copyFileSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const src = new URL('../src/', import.meta.url)
const toolchain = `go${readFileSync(`${repo}go.mod`, 'utf8').match(/^go (\S+)$/m)[1]}`
const go = (args, env = {}) =>
  execFileSync('go', args, {
    cwd: repo,
    env: { ...process.env, GOTOOLCHAIN: toolchain, ...env },
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'inherit'],
  }).trim()

const wasm = fileURLToPath(new URL('game/player/predictor.wasm', src))
const sim = go(['run', './shards/game/predictor/version'])
go(['build', '-trimpath', `-ldflags=-s -w -X main.simVersion=${sim}`, '-o', wasm, './shards/game/predictor'], {
  GOOS: 'js',
  GOARCH: 'wasm',
})
copyFileSync(`${go(['env', 'GOROOT'])}/lib/wasm/wasm_exec.js`, new URL('engine/wasm/wasm_exec.js', src))

/** vitest's global setup: the build above has run by now, and tests can inject its simVersion. */
export default function setup(project) {
  const run = fileURLToPath(new URL('game/player/prediction.msgpack', src))
  go(['test', './shards/game', '-run', '^TestPredictionMatchesServer$', '-count=1', `-record=${run}`])
  project.provide('simVersion', sim)
}
