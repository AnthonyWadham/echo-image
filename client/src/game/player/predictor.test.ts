import { readFileSync } from 'node:fs'
import { decodePayload } from '@argus-labs/sdk'
import { beforeAll, expect, inject, test } from 'vitest'
import type { TargetSync } from '../projectile/target_sync_event'
import type { BodySync } from './body_sync_event'
import type { InputFrame } from './input_command'
import { loadPredictor } from './predictor'
import type { EndedShot, PredictedEcho, PredictedKit, PredictedShot, Predictor } from './predictor'

// TestPredictionMatchesServer's run (shards/game/prediction_test.go), which scripts/predictor.mjs has
// it write before the tests, at the wasm's rate: the body_sync and target_sync before each tick, the
// frames her client stepped from them, and what the Go predictor gave after the last: the server's
// state once it applies that frame, unless it coasted or dropped one first. Casts, a dash, a shot
// flying out its range, slows, a stun, bad frames, a shot that stops on bob where the frame's view drew
// him, her Echo retracing her path, and a bad network's lost, late and held Inputs are in it.
interface Step {
  Sync: BodySync
  Targets: TargetSync
  Frames: InputFrame[]
  X: number
  Y: number
  Shots: PredictedShot[]
  Ended: EndedShot[]
  Kit: PredictedKit
  Echo: PredictedEcho | null // null with no Echo
}
// The content files it played in place of the shipped ones, which the wasm build plays too.
interface Recording {
  Content: Record<string, string>
  Steps: Step[]
}
const recording = readFileSync(new URL('prediction.msgpack', import.meta.url))
const { Content: content, Steps: run } = decodePayload(new Uint8Array(recording)) as Recording

declare module 'vitest' {
  export interface ProvidedContext {
    simVersion: string // the tree's echoimage.SimVersion, from scripts/predictor.mjs
  }
}

let predictor: Predictor
beforeAll(async () => {
  // Built by scripts/predictor.mjs, vitest's global setup.
  predictor = await loadPredictor(readFileSync(new URL('predictor.wasm', import.meta.url)), content)
})

// Steps each frame in turn, and returns where the last left her and every shot the steps ended.
const stepAll = (frames: InputFrame[]) => {
  const steps = frames.map((f) => predictor.step(f))
  return { at: steps.at(-1)?.at, ended: steps.flatMap((s) => s.ended) }
}

test("plays the run's content: its render delay too", () => {
  const projectile = JSON.parse(content['projectile/tuning.json']!) as { rewind: { interpolation_seconds: number } }
  expect(predictor.interpolationSeconds).toBe(projectile.rewind.interpolation_seconds)
})

test("runs this tree's sim: the version a shard built from it sends", () => {
  expect(predictor.version).toMatch(/^go[\d.]+(\+[\w,]+)?\+[0-9a-f]{16}$/)
  expect(predictor.version).toBe(inject('simVersion'))
  // Stamped with the tags it's built with, which pick the recorded server's rate.
  expect(predictor.version.includes('+rate60+')).toBe(Math.round(1 / run[0]!.Sync.TickSeconds) === 60)
})

test('the wasm build lands on the server bits each tick, ended shots included, and so does a replay', () => {
  expect(run.length).toBeGreaterThan(0)
  // Shots end out of range and on bob, after one frame and after several (her replays on the bad network).
  const ends = new Set(run.flatMap((s) => s.Ended.map((e) => `${s.Frames.length > 1} ${e.OnTarget}`)))
  expect(ends).toStrictEqual(new Set(['false false', 'false true', 'true false', 'true true']))
  expect(new Set(run.map((s) => s.Echo?.X)).size).toBeGreaterThan(1) // her Echo moves
  predictor.forget()
  for (const [i, s] of run.entries()) {
    const you = s.Sync.Bodies.find((b) => b.EntityID === s.Sync.You.Body)
    if (!you) throw new Error(`step ${i} has no Body ${s.Sync.You.Body}`)
    predictor.seeTargets(s.Targets)
    predictor.reset(s.Sync, you)
    // Until a step, each shot is drawn flying the tick the server flew.
    const synced = s.Sync.You.Shots.map(({ VelX, VelY, Remaining, ...shot }) => shot)
    expect(predictor.shots(), `step ${i} reset`).toStrictEqual(synced)
    const stepped = { at: { x: s.X, y: s.Y }, ended: s.Ended }
    expect(stepAll(s.Frames), `step ${i}`).toStrictEqual(stepped)
    expect(predictor.shots(), `step ${i} shots`).toStrictEqual(s.Shots)
    expect(predictor.kit(), `step ${i} kit`).toStrictEqual(s.Kit)
    expect(predictor.echo(), `step ${i} echo`).toStrictEqual(s.Echo ?? undefined)
    predictor.reset(s.Sync, you)
    expect(stepAll(s.Frames), `step ${i} replayed`).toStrictEqual(stepped)
    expect(predictor.shots(), `step ${i} replayed shots`).toStrictEqual(s.Shots)
    expect(predictor.echo(), `step ${i} replayed echo`).toStrictEqual(s.Echo ?? undefined)
  }
})

test('agrees with a body_sync only where its own step left your Body', () => {
  const [s, next] = [run[0]!, run[1]!] // the server applied s's frame, with no surprise
  const body = (x: Step) => x.Sync.Bodies.find((b) => b.EntityID === x.Sync.You.Body)!
  for (const [you, agrees] of [
    [{ ...body(next), X: body(next).X + 1e-9 }, false],
    [body(next), true],
  ] as const) {
    predictor.forget()
    predictor.seeTargets(s.Targets)
    predictor.reset(s.Sync, body(s))
    stepAll(s.Frames)
    expect(predictor.agrees(next.Sync, you)).toBe(agrees)
  }
})

test("effects keep their stacking through the wasm glue: two sources' per-source hastes add", () => {
  const s = run[0]!
  const you = s.Sync.Bodies.find((b) => b.EntityID === s.Sync.You.Body)!
  const haste = (Source: number) => ({
    ...{ Kind: 'haste', Tags: 0n, Pass: 0n, Stat: 1, Value: 5000, VelX: 0, VelY: 0, Source },
    ...{ Start: 0n, End: s.Sync.Tick + 100n, Serial: Source, Stacking: 1 /* status.PerSource */ },
  })
  predictor.forget()
  predictor.reset({ ...s.Sync, You: { ...s.Sync.You, Effects: [haste(7), haste(8)], EffectsNext: 8 } }, you)
  const { at } = predictor.step({ ...s.Frames[0]!, MoveX: 1, MoveY: 0, Buttons: 0 })
  expect(at.x - you.X).toBeCloseTo(s.Sync.You.MoveSpeed * 2 * s.Sync.TickSeconds, 9)
})

test('a shot ends the same after a step as after a reset: from where its last tick found it', () => {
  const last = run.findIndex((s) => s.Ended.length > 0) // her first bolt, out of its range
  const before = run[last - 1]
  const you = before?.Sync.Bodies.find((b) => b.EntityID === before.Sync.You.Body)
  if (!before || !you) throw new Error('the run has no shot flying out its range')
  predictor.forget()
  predictor.reset(before.Sync, you)
  stepAll(before.Frames)
  expect(stepAll(run[last]!.Frames).ended).toStrictEqual(run[last]!.Ended)
})

test('an Echo is hit where your screen drew it, unless it died before your frame', () => {
  const first = run.find((s) => s.Shots.length > s.Sync.You.Shots.length && s.Sync.Tick >= 2n)
  const you = first?.Sync.Bodies.find((b) => b.EntityID === first.Sync.You.Body)
  const [cast] = first?.Frames ?? []
  if (!first || !you || !cast) throw new Error('the run has no cast')
  // An enemy Echo on your aim, touching your shot as it leaves, in the two syncs before a tick you cast in,
  // drawn alive. It lived all of the newer tick, or died halfway through it: then the server removed it
  // before your frame's tick.
  const echo = {
    ...{ EntityID: 99, Serial: 99n, Kind: 4n /* interact.Echo */, Owner: 98, Team: 1 - you.Team },
    ...{ X: you.X + 0.5, Y: you.Y, Radius: 0.5, Tags: 0n },
  }
  const older = first.Sync.Tick - 2n
  for (const [until, hits] of [
    [1, true],
    [0.5, false],
  ] as const) {
    predictor.forget()
    for (const [tick, lived] of [
      [older, 1],
      [older + 1n, until],
    ] as const) {
      predictor.seeTargets({ Tick: tick, Targets: [{ ...echo, Until: lived }] })
    }
    predictor.reset(first.Sync, you)
    const frame = { ...cast, MoveX: 0, MoveY: 0, AimX: 1, AimY: 0, Buttons: 1 }
    const { ended } = predictor.step({ ...frame, ViewTick: Number(older) + 0.25 })
    // Your shot ends on it in its first tick, or flies on.
    expect(ended, `an Echo that lived ${until} of its tick`).toHaveLength(hits ? 1 : 0)
  }
})
