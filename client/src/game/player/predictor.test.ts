import { readFileSync } from 'node:fs'
import { decodePayload } from '@argus-labs/sdk'
import { beforeAll, expect, test } from 'vitest'
import type { BodySync } from './body_sync_event'
import type { InputFrame } from './input_command'
import { loadPredictor } from './predictor'
import type { PredictedKit, PredictedShot, Predictor } from './predictor'

// TestPredictionMatchesServer's run (shards/game/main_test.go): the body_sync before each tick, the
// frame the server applied, and what the Go predictor gave after it, which matched the server. Casts,
// a dash, a shot flying out its range, slows, a stun and bad frames are in it.
interface Step {
  Sync: BodySync
  Frame: InputFrame
  X: number
  Y: number
  Shots: PredictedShot[]
  Kit: PredictedKit
}
const recording = new URL('../../../../shards/game/testdata/prediction.msgpack', import.meta.url)
const run = decodePayload(new Uint8Array(readFileSync(recording))) as Step[]

let predictor: Predictor
beforeAll(async () => {
  // Built by scripts/predictor.mjs, vitest's global setup.
  predictor = await loadPredictor(readFileSync(new URL('predictor.wasm', import.meta.url)))
})

test('the wasm build lands on the server bits each tick', () => {
  expect(run.length).toBeGreaterThan(0)
  for (const [i, s] of run.entries()) {
    const you = s.Sync.Bodies.find((b) => b.EntityID === s.Sync.You.Body)
    if (!you) throw new Error(`step ${i} has no Body ${s.Sync.You.Body}`)
    predictor.reset(s.Sync, you)
    // Until a step, each shot is drawn where the server has it.
    const synced = s.Sync.You.Shots.map(({ Type, X, Y, Radius }) => ({ Type, X, Y, PrevX: X, PrevY: Y, Radius }))
    expect(predictor.shots(), `step ${i} reset`).toStrictEqual(synced)
    expect(predictor.step(s.Frame), `step ${i}`).toStrictEqual({ x: s.X, y: s.Y })
    expect(predictor.shots(), `step ${i} shots`).toStrictEqual(s.Shots)
    expect(predictor.kit(), `step ${i} kit`).toStrictEqual(s.Kit)
  }
})
