import { describe, expect, test } from 'vitest'
import type { BodyState, BodySync } from './body_sync_event'
import type { InputFrame } from './input_command'
import type { FrameInput } from './input_stream'
import { Prediction } from './prediction'
import type { PredictedShot, Predictor } from './predictor'

const tickSeconds = 0.05
const keep = Math.exp(-tickSeconds / 0.1) // what a tick leaves of a correction

/** A stand-in for the wasm: each frame moves the Body by its move, and button 0 casts a shot that flies up 1 a tick. */
function predictor(): Predictor {
  let at = { x: 0, y: 0 }
  let tick = 0
  let shots: PredictedShot[] = []
  return {
    reset: (sync, you) => {
      at = { x: you.X, y: you.Y }
      tick = Number(sync.Tick)
      shots = sync.You.Shots.map(({ Type, X, Y, Radius }) => ({ Type, X, Y, PrevX: X, PrevY: Y, Radius }))
    },
    peek: (x, y) => ({ x: at.x + x, y: at.y + y }),
    step: (f) => {
      tick++
      at = { x: at.x + f.MoveX, y: at.y + f.MoveY }
      if (f.Buttons & 1) shots.push({ Type: 'bolt', X: at.x, Y: at.y, PrevX: at.x, PrevY: at.y, Radius: 0.25 })
      shots = shots.map((s) => ({ ...s, PrevX: s.X, PrevY: s.Y, Y: s.Y + 1 }))
      return at
    },
    shots: () => shots,
    kit: () => ({ Tick: tick, Slots: [{ Ability: 'bolt', ReadyTick: 0 }] }),
  }
}

const you = (x: number): BodyState => ({ EntityID: 1, PlayerID: 1, Nickname: 'P1', Team: 0, X: x, Y: 0, Radius: 0.5 })
const sync = (x: number): BodySync => ({
  Tick: 100n,
  TickSeconds: tickSeconds,
  You: {
    Body: 1,
    InputSession: 7,
    InputAck: 0,
    MoveSpeed: 5,
    Champion: 'starter',
    Cooldowns: [],
    Effects: [],
    EffectsNext: 0,
    Shots: [],
  },
  Bodies: [you(x)],
})
const move = (x: number, buttons = 0): FrameInput => ({ MoveX: x, MoveY: 0, AimX: 0, AimY: 0, Buttons: buttons })
const right = (seq: number): InputFrame => ({ Seq: seq, ...move(1) })

/** At x = 2 after frames 1 and 2, with the server at 0 and neither applied. */
function ahead(): Prediction {
  const p = new Prediction(predictor())
  p.reconcile(sync(0), you(0), [])
  p.step(move(1), tickSeconds)
  p.step(move(1), tickSeconds)
  return p
}

describe('Prediction', () => {
  test('moves your Body at once, and draws it through the newest tick', () => {
    const p = ahead()
    expect(p.position).toEqual({ x: 2, y: 0 })
    expect(p.drawn(0)).toEqual({ x: 1, y: 0 })
    expect(p.drawn(0.5)).toEqual({ x: 1.5, y: 0 })
  })

  test('replays the frames a body_sync has not applied, and an exact replay is no correction', () => {
    const p = ahead()
    p.reconcile(sync(1), you(1), [right(2)]) // the server applied frame 1
    expect(p.position).toEqual({ x: 2, y: 0 })
    expect(p.corrections).toBe(0)
    expect(p.drawn(0.5)).toEqual({ x: 1.5, y: 0 })
  })

  test('a correction keeps the drawn Body in place, then fades', () => {
    const p = ahead()
    p.reconcile(sync(0.5), you(0.5), [right(2)]) // the server is half a unit behind
    expect(p.corrections).toBe(1)
    expect(p.position).toEqual({ x: 1.5, y: 0 })
    expect(p.drawn(0.5)).toEqual({ x: 1.5, y: 0 })
    p.step(move(1), tickSeconds)
    expect(p.drawn(1).x).toBeCloseTo(2.5 + 0.5 * keep)
    for (let i = 0; i < 30; i++) p.step(move(0), tickSeconds)
    expect(p.drawn(1).x).toBeCloseTo(2.5)
  })

  test('a big correction jumps', () => {
    const p = ahead()
    p.reconcile(sync(10), you(10), [])
    expect(p.drawn(0)).toEqual({ x: 10, y: 0 })
    expect(p.drawn(1)).toEqual({ x: 10, y: 0 })
  })

  test('starts over after clear', () => {
    const p = ahead()
    p.step(move(0, 1), tickSeconds)
    p.clear()
    expect(p.position).toBeUndefined()
    expect(() => p.step(move(1), tickSeconds)).toThrow()
    expect(() => p.peek(1, 0)).toThrow()
    expect(p.shots(0)).toEqual([])
    expect(p.kit).toBeUndefined()
    p.reconcile(sync(5), you(5), [])
    expect(p.corrections).toBe(0)
    expect(p.drawn(0.5)).toEqual({ x: 5, y: 0 })
  })

  test('draws a shot from where you cast it, through the newest tick', () => {
    const p = ahead()
    p.step(move(0, 1), tickSeconds) // casts at x = 2
    expect(p.shots(0)).toEqual([{ Type: 'bolt', X: 2, Y: 0, Radius: 0.25 }])
    expect(p.shots(0.5)).toEqual([{ Type: 'bolt', X: 2, Y: 0.5, Radius: 0.25 }])
    expect(p.kit).toEqual({ Tick: 103, Slots: [{ Ability: 'bolt', ReadyTick: 0 }] })
  })

  test('rebuilds your shots from each body_sync', () => {
    const p = ahead()
    p.step(move(0, 1), tickSeconds)
    p.reconcile(sync(2), you(2), []) // the server ended the shot
    expect(p.shots(1)).toEqual([])
    const s = sync(2)
    s.You.Shots = [{ Type: 'bolt', X: 2, Y: 3, VelX: 0, VelY: 1, Radius: 0.25, Remaining: 4 }]
    p.reconcile(s, you(2), [right(4)])
    expect(p.shots(1)).toEqual([{ Type: 'bolt', X: 2, Y: 4, Radius: 0.25 }])
  })
})
