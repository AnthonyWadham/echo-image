import { readFileSync } from 'node:fs'
import { decodePayload } from '@argus-labs/sdk'
import { beforeAll, expect, inject, test } from 'vitest'
import type { TargetSync } from '../history/target_sync_event'
import type { BodySync } from './body_sync_event'
import type { InputFrame } from './input_command'
import type { MapFile } from '../arena/map_view'
import { loadPredictor } from './predictor'
import type { EndedShot, PredictedEcho, PredictedKit, PredictedShot, Predictor } from './predictor'

// TestPredictionMatchesServer's run (shards/game/prediction_test.go), which scripts/predictor.mjs has
// it write before the tests, at the wasm's rate: the body_sync and target_sync before each tick, the
// frames her client stepped from them, and what the Go predictor gave after the last: the server's
// state once it applies that frame, unless it coasted or dropped one first. Casts, a dash into a wall
// under a phase and a landing out of it, slides along a wall and over the map edge's joints, shots
// ending on a wall, on the edge and out of their range, slows, a stun, bad frames, a shot that stops on
// bob where the frame's view drew him, her Echo retracing her path, and a bad network's lost, late and
// held Inputs are in it.
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

// Built by scripts/predictor.mjs, vitest's global setup.
const wasm = readFileSync(new URL('predictor.wasm', import.meta.url))
let predictor: Predictor
beforeAll(async () => {
  predictor = await loadPredictor(wasm, content)
})

// Steps each frame in turn, and returns where the last left her and every shot the steps ended.
const stepAll = (frames: InputFrame[]) => {
  const steps = frames.map((f) => predictor.step(f))
  return { at: steps.at(-1)?.at, ended: steps.flatMap((s) => s.ended) }
}

test("plays the run's content: its render delay too", () => {
  const history = JSON.parse(content['history/tuning.json']!) as { window: { interpolation_seconds: number } }
  expect(predictor.interpolationSeconds).toBe(history.window.interpolation_seconds)
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
    predictor.seeTargets(s.Targets)
    predictor.reset(s.Sync)
    // Until a step, each shot is drawn flying the tick the server flew.
    const synced = s.Sync.You.Shots.map(({ VelX, VelY, Remaining, ...shot }) => shot)
    expect(predictor.shots(), `step ${i} reset`).toStrictEqual(synced)
    const stepped = { at: { x: s.X, y: s.Y }, ended: s.Ended }
    expect(stepAll(s.Frames), `step ${i}`).toStrictEqual(stepped)
    expect(predictor.shots(), `step ${i} shots`).toStrictEqual(s.Shots)
    expect(predictor.kit(), `step ${i} kit`).toStrictEqual(s.Kit)
    expect(predictor.echo(), `step ${i} echo`).toStrictEqual(s.Echo ?? undefined)
    predictor.reset(s.Sync)
    expect(stepAll(s.Frames), `step ${i} replayed`).toStrictEqual(stepped)
    expect(predictor.shots(), `step ${i} replayed shots`).toStrictEqual(s.Shots)
    expect(predictor.echo(), `step ${i} replayed echo`).toStrictEqual(s.Echo ?? undefined)
  }
})

test('agrees with a body_sync only where its own step left your Body', () => {
  const [s, next] = [run[0]!, run[1]!] // the server applied s's frame, with no surprise
  for (const [sync, agrees] of [
    [{ ...next.Sync, You: { ...next.Sync.You, X: next.Sync.You.X + 1e-9 } }, false],
    [{ ...next.Sync, You: { ...next.Sync.You, EchoSerial: next.Sync.You.EchoSerial + 1n } }, false],
    [next.Sync, true],
  ] as const) {
    predictor.forget()
    predictor.seeTargets(s.Targets)
    predictor.reset(s.Sync)
    stepAll(s.Frames)
    expect(predictor.agrees(sync)).toBe(agrees)
  }
})

test("effects keep their stacking through the wasm glue: two sources' per-source hastes add", () => {
  const s = run[0]!
  const you = s.Sync.You
  const haste = (Source: number) => ({
    ...{ Kind: 'haste', Tags: 0n, Pass: 0n, Stat: 1, Value: 5000, VelX: 0, VelY: 0, Source },
    ...{ Start: 0n, End: s.Sync.Tick + 100n, Serial: Source, Stacking: 1 /* status.PerSource */ },
  })
  predictor.forget()
  predictor.reset({ ...s.Sync, You: { ...s.Sync.You, Effects: [haste(7), haste(8)], EffectsNext: 8 } })
  const { at } = predictor.step({ ...s.Frames[0]!, MoveX: 1, MoveY: 0, Buttons: 0 })
  expect(at.x - you.X).toBeCloseTo(s.Sync.You.MoveSpeed * 2 * s.Sync.TickSeconds, 9)
})

test('a shot ends the same after a step as after a reset: from where its last tick found it', () => {
  const last = run.findIndex((s) => s.Ended.length > 0) // her first bolt, out of its range
  const before = run[last - 1]
  if (!before) throw new Error('the run has no shot flying out its range')
  predictor.forget()
  predictor.reset(before.Sync)
  stepAll(before.Frames)
  expect(stepAll(run[last]!.Frames).ended).toStrictEqual(run[last]!.Ended)
})

test('an Echo is hit where your screen drew it, unless it died before your frame', () => {
  const first = run.find((s) => s.Shots.length > s.Sync.You.Shots.length && s.Sync.Tick >= 2n)
  const you = first?.Sync.You
  const [cast] = first?.Frames ?? []
  if (!first || !you || !cast) throw new Error('the run has no cast')
  // An enemy Echo on your aim, touching your shot as it leaves, in the three syncs the views of a tick you
  // cast in are drawn from, drawn alive. It lived all of the middle tick, or died halfway through it: then
  // the server removed it at that tick's end, before your frame's tick, and the newest sync lacks it.
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
    const synced = [[{ ...echo, Until: 1 }], [{ ...echo, Until: until }], until < 1 ? [] : [{ ...echo, Until: 1 }]]
    synced.forEach((targets, i) => predictor.seeTargets({ Tick: older + BigInt(i), Targets: targets }))
    predictor.reset(first.Sync)
    const frame = { ...cast, MoveX: 0, MoveY: 0, AimX: 1, AimY: 0, Buttons: 1 }
    const { ended } = predictor.step({ ...frame, ViewTick: Number(older) + 0.25 })
    // Your shot ends on it in its first tick, or flies on.
    expect(ended, `an Echo that lived ${until} of its tick`).toHaveLength(hits ? 1 : 0)
  }
})

test("gives a map's file as the server decoded it, with its bounds, and none for a map it doesn't have", async () => {
  // The run's map: a polygon, an outline and an edge.
  const { default_map: id } = JSON.parse(content['arena/tuning.json']!) as { default_map: string }
  const file = JSON.parse(content[`arena/${id}.level.json`]!) as MapFile
  expect(file.solids.length * file.outlines.length * file.edges.length).toBeGreaterThan(0)
  expect(predictor.shapes(id)?.File).toStrictEqual(file)
  expect(predictor.shapes('no such map')).toBeUndefined()
  // A round wall and a capsule, in a predictor of their own.
  const round = [
    '{',
    '  "format": 1,',
    '  "solids": [',
    '    {"src": 1, "kind": "wall", "circle": {"center": [2048, 3072], "radius": 512}},',
    '    {"src": 2, "kind": "wall", "capsule": {"center1": [-4096, 0], "center2": [-2048, 1024], "radius": 256}}',
    '  ],',
    '  "outlines": [],',
    '  "edges": [',
    '    {"src": 3, "kind": "bounds", "points": [[-10240, -10240], [-10240, 10240], [10240, 10240], [10240, -10240]]}',
    '  ],',
    '  "objects": []',
    '}',
    '',
  ].join('\n')
  const other = await loadPredictor(wasm, {
    'arena/tuning.json': JSON.stringify({ maps: ['round'], default_map: 'round' }),
    'arena/round.level.json': round,
  })
  expect(other.shapes('round')).toStrictEqual({
    File: JSON.parse(round) as MapFile,
    Bounds: { min: { x: -10, y: -10 }, max: { x: 10, y: 10 } }, // Go's level.Map.Bounds
  })
})
