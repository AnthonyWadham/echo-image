import { readFileSync } from 'node:fs'
import { decodePayload } from '@argus-labs/sdk'
import { beforeAll, expect, test } from 'vitest'
import type { EchoSync } from '../echo/echo_sync_event'
import type { BodySync } from './body_sync_event'
import type { InputFrame } from './input_command'
import { loadPredictor } from './predictor'
import type { EndedShot, PredictedKit, PredictedShot, Predictor } from './predictor'

// TestPredictionMatchesServer's run (shards/game/main_test.go): the body_sync and echo_sync before
// each tick, the frame the server applied with the Go predictor's claims, and what the Go predictor
// gave after it, which matched the server. Casts, a dash, a shot flying out its range, slows, a stun,
// bad frames and a shot that stops on bob where the frame's view drew him are in it.
interface Step {
  Sync: BodySync
  Echoes: EchoSync
  Frame: InputFrame
  X: number
  Y: number
  Shots: PredictedShot[]
  Ended: EndedShot[]
  Kit: PredictedKit
}
const recording = new URL('../../../../shards/game/testdata/prediction.msgpack', import.meta.url)
const run = decodePayload(new Uint8Array(readFileSync(recording))) as Step[]

let predictor: Predictor
beforeAll(async () => {
  // Built by scripts/predictor.mjs, vitest's global setup.
  predictor = await loadPredictor(readFileSync(new URL('predictor.wasm', import.meta.url)))
})

test('the wasm build lands on the server bits each tick, claims and ended shots included', () => {
  expect(run.length).toBeGreaterThan(0)
  expect(run.filter((s) => s.Frame.Claims.length > 0)).toHaveLength(1)
  expect(run.flatMap((s) => s.Ended)).toHaveLength(2) // one out of range, one on bob
  predictor.forget()
  for (const [i, s] of run.entries()) {
    const you = s.Sync.Bodies.find((b) => b.EntityID === s.Sync.You.Body)
    if (!you) throw new Error(`step ${i} has no Body ${s.Sync.You.Body}`)
    predictor.seeBodies(s.Sync)
    predictor.seeEchoes(s.Echoes)
    predictor.reset(s.Sync, you)
    // Until a step, each shot is drawn where the server has it.
    const synced = s.Sync.You.Shots.map(({ Type, X, Y, Radius }) => ({ Type, X, Y, PrevX: X, PrevY: Y, Radius }))
    expect(predictor.shots(), `step ${i} reset`).toStrictEqual(synced)
    const stepped = { at: { x: s.X, y: s.Y }, claims: s.Frame.Claims, ended: s.Ended }
    expect(predictor.step(s.Frame), `step ${i}`).toStrictEqual(stepped)
    expect(predictor.shots(), `step ${i} shots`).toStrictEqual(s.Shots)
    expect(predictor.kit(), `step ${i} kit`).toStrictEqual(s.Kit)
    // Replayed, the frame ends the shots it claimed.
    predictor.reset(s.Sync, you)
    expect(predictor.replay(s.Frame), `step ${i} replayed`).toStrictEqual({ x: s.X, y: s.Y })
    expect(predictor.shots(), `step ${i} replayed shots`).toStrictEqual(s.Shots)
  }
})

test('a shot ends the same after a step as after a reset: from where its last tick found it', () => {
  const last = run.findIndex((s) => s.Ended.length > 0 && s.Frame.Claims.length === 0) // out of its range
  const before = run[last - 1]
  const you = before?.Sync.Bodies.find((b) => b.EntityID === before.Sync.You.Body)
  if (!before || !you) throw new Error('the run has no shot flying out its range')
  predictor.forget()
  predictor.reset(before.Sync, you)
  predictor.step(before.Frame)
  expect(predictor.step(run[last]!.Frame).ended).toStrictEqual(run[last]!.Ended)
})

test('an Echo that died partway through a tick stops your shot only until then', () => {
  const first = run.find((s) => s.Shots.length > s.Sync.You.Shots.length && s.Sync.Tick >= 2n)
  const you = first?.Sync.Bodies.find((b) => b.EntityID === first.Sync.You.Body)
  if (!first || !you) throw new Error('the run has no cast')
  // An enemy Echo on your aim, in the two syncs before a tick you cast in. It died halfway through the newer.
  const echo = { EntityID: 99, PlayerID: 9, Owner: 98, Team: 1 - you.Team, X: you.X + 1, Y: you.Y, Radius: 0.5 }
  const older = first.Sync.Tick - 2n
  for (const [part, hits] of [
    [0.25, true],
    [0.75, false],
  ] as const) {
    predictor.forget()
    for (const [tick, until] of [
      [older, 1],
      [older + 1n, 0.5],
    ] as const) {
      predictor.seeBodies({ ...first.Sync, Tick: tick, Bodies: [] })
      const dying = { ...echo, Tags: 0n, HP: until < 1 ? 0 : 100, MaxHP: 100, Until: until }
      predictor.seeEchoes({ Tick: tick, Echoes: [dying] })
    }
    predictor.reset(first.Sync, you)
    const frame = { ...first.Frame, MoveX: 0, MoveY: 0, AimX: 1, AimY: 0, Buttons: 1, Claims: [] }
    const { claims } = predictor.step({ ...frame, ViewTick: Number(older) + part })
    expect(claims.map((c) => c.Target), `seen ${part} through the tick`).toStrictEqual(hits ? [echo.EntityID] : [])
  }
})
