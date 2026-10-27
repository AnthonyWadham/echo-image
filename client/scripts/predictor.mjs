// Builds the client predictor (shards/game/predictor) to wasm with the Go toolchain the server is built
// with, and copies that toolchain's wasm_exec.js next to its loader, so the two always match. The dev
// and build scripts run it first, and vitest runs it as its global setup.
import { execFileSync } from 'node:child_process'
import { copyFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const repo = fileURLToPath(new URL('../../', import.meta.url))
const src = new URL('../src/', import.meta.url)
const go = (args, env = {}) =>
  execFileSync('go', args, {
    cwd: repo,
    env: { ...process.env, ...env },
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'inherit'],
  }).trim()

const wasm = fileURLToPath(new URL('game/player/predictor.wasm', src))
go(['build', '-trimpath', '-ldflags=-s -w', '-o', wasm, './shards/game/predictor'], { GOOS: 'js', GOARCH: 'wasm' })
copyFileSync(`${go(['env', 'GOROOT'])}/lib/wasm/wasm_exec.js`, new URL('engine/wasm/wasm_exec.js', src))

/** vitest's global setup: the build above has run by now. */
export default function setup() {}
