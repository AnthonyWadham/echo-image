import { describe, expect, test } from 'vitest'
import type { BodyState, BodySync, ShotState } from './body_sync_event'
import type { HitClaim, InputFrame } from './input_command'
import { Prediction } from './prediction'
import type { PredictedShot, Predictor } from './predictor'

const tickSeconds = 0.05
const keep = Math.exp(-tickSeconds / 0.1) // what a tick leaves of a correction

type Shot = PredictedShot & Pick<HitClaim, 'Seq' | 'Ability' | 'Launch'>

/**
 * A stand-in for the wasm: each frame moves the Body by its move, and button 0 casts a shot that flies
 * up 1 a tick. A new frame with a view ends every shot in flight on Body 9, half way up its tick: its
 * claims.
 */
function predictor(): Predictor {
  let at = { x: 0, y: 0 }
  let tick = 0
  let shots: Shot[] = []
  const run = (f: InputFrame, ended: readonly HitClaim[]) => {
    tick++
    at = { x: at.x + f.MoveX, y: at.y + f.MoveY }
    shots = shots.filter((s) => !ended.some((c) => c.Seq === s.Seq && c.Ability === s.Ability && c.Launch === s.Launch))
    if (f.Buttons & 1) {
      const key = { Seq: f.Seq, Ability: 'bolt', Launch: 0 }
      shots.push({ Type: 'bolt', ...key, X: at.x, Y: at.y, PrevX: at.x, PrevY: at.y, Radius: 0.25 })
    }
    shots = shots.map((s) => ({ ...s, PrevX: s.X, PrevY: s.Y, Y: s.Y + 1 }))
  }
  return {
    reset: (sync, you) => {
      at = { x: you.X, y: you.Y }
      tick = Number(sync.Tick)
      shots = sync.You.Shots.map(({ Type, Seq, Ability, Launch, X, Y, Radius }) => ({
        Type, Seq, Ability, Launch, X, Y, PrevX: X, PrevY: Y, Radius,
      }))
    },
    seeBodies: () => {},
    seeEchoes: () => {},
    forget: () => {},
    peek: (x, y) => ({ x: at.x + x, y: at.y + y }),
    step: (f) => {
      const hit = ({ Seq, Ability, Launch }: Shot): HitClaim => ({ Seq, Ability, Launch, Target: 9 })
      const claims = f.ViewTick > 0 ? shots.map(hit) : []
      const end = ({ Type, X, Y, Radius }: Shot) => ({ Type, X, Y: Y + 0.5, PrevX: X, PrevY: Y, Radius, Until: 0.5 })
      const ended = f.ViewTick > 0 ? shots.map(end) : []
      run(f, claims)
      return { at, claims, ended }
    },
    replay: (f) => {
      run(f, f.Claims)
      return at
    },
    shots: () => shots.map(({ Type, X, Y, PrevX, PrevY, Radius }) => ({ Type, X, Y, PrevX, PrevY, Radius })),
    kit: () => ({ Tick: tick, Slots: [{ Ability: 'bolt', ReadyTick: 0 }] }),
  }
}

const you = (x: number): BodyState => ({
  EntityID: 1,
  PlayerID: 1,
  Nickname: 'P1',
  Team: 0,
  X: x,
  Y: 0,
  Radius: 0.5,
  Tags: 0n,
})
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
const move = (x: number, buttons = 0, seq = 0): InputFrame => ({
  Seq: seq,
  MoveX: x,
  MoveY: 0,
  AimX: 0,
  AimY: 0,
  Buttons: buttons,
  ViewTick: 0,
  Claims: [],
})
const right = (seq: number): InputFrame => move(1, 0, seq)

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
    p.step({ ...move(0), ViewTick: 97.5 }, tickSeconds) // ends the shot
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
    s.You.Shots = [
      { Type: 'bolt', Ability: 'bolt', Seq: 3, Launch: 0, X: 2, Y: 3, VelX: 0, VelY: 1, Radius: 0.25, Remaining: 4 },
    ]
    p.reconcile(s, you(2), [right(4)])
    expect(p.shots(1)).toEqual([{ Type: 'bolt', X: 2, Y: 4, Radius: 0.25 }])
  })

  test('a shot your frame ended stays hidden while a frame claims it or the server lists it', () => {
    const p = new Prediction(predictor())
    p.reconcile(sync(0), you(0), [])
    const cast = move(0, 1, 1)
    p.step(cast, tickSeconds)
    const hit: InputFrame = { ...move(0, 0, 2), ViewTick: 97.5 }
    const claims = p.step(hit, tickSeconds)
    expect(claims).toEqual([{ Seq: 1, Ability: 'bolt', Launch: 0, Target: 9 }])
    expect(p.shots(1)).toEqual([])
    p.reconcile(sync(0), you(0), [cast, { ...hit, Claims: claims }]) // the server has neither frame
    expect(p.shots(1)).toEqual([])
    const listed = sync(0)
    const shot: ShotState = {
      ...{ Type: 'bolt', Ability: 'bolt', Seq: 1, Launch: 0 },
      ...{ X: 0, Y: 1, VelX: 0, VelY: 1, Radius: 0.25, Remaining: 4 },
    }
    listed.You.Shots = [shot]
    p.reconcile(listed, you(0), [{ ...hit, Claims: claims }]) // the server hasn't applied the claim yet
    expect(p.shots(1)).toEqual([])
    p.reconcile(listed, you(0), []) // it rejected the claim, and its shot flies on
    expect(p.shots(1)).toEqual([])
    p.reconcile(sync(0), you(0), []) // then that shot is gone too
    p.reconcile(listed, you(0), []) // so a shot with its key would show
    expect(p.shots(1)).toEqual([{ Type: 'bolt', X: 0, Y: 1, Radius: 0.25 }])
  })

  test('a shot that ended in the newest tick flies on to where it ended, even after a body_sync', () => {
    const p = new Prediction(predictor())
    p.reconcile(sync(0), you(0), [])
    const cast = move(0, 1, 1)
    p.step(cast, tickSeconds) // the shot flies up to y = 1
    const hit: InputFrame = { ...move(0, 0, 2), ViewTick: 97.5 }
    const claims = p.step(hit, tickSeconds) // and ends at y = 1.5, half way through the tick
    expect(p.shots(0)).toEqual([{ Type: 'bolt', X: 0, Y: 1, Radius: 0.25 }])
    expect(p.shots(0.25)).toEqual([{ Type: 'bolt', X: 0, Y: 1.25, Radius: 0.25 }])
    expect(p.shots(0.5)).toEqual([{ Type: 'bolt', X: 0, Y: 1.5, Radius: 0.25 }])
    expect(p.shots(0.6)).toEqual([])
    p.reconcile(sync(0), you(0), [cast, { ...hit, Claims: claims }])
    expect(p.shots(0.25)).toEqual([{ Type: 'bolt', X: 0, Y: 1.25, Radius: 0.25 }])
    p.step(move(0, 0, 3), tickSeconds)
    expect(p.shots(0)).toEqual([])
  })

  test('a replay ends the shots its frame claimed, and claims none', () => {
    const p = new Prediction(predictor())
    p.reconcile(sync(0), you(0), [])
    const cast = move(0, 1, 1)
    p.step(cast, tickSeconds)
    const viewed: InputFrame = { ...move(0, 0, 2), ViewTick: 97.5, Claims: [] }
    p.reconcile(sync(0), you(0), [cast, viewed])
    expect(p.shots(1)).toEqual([{ Type: 'bolt', X: 0, Y: 2, Radius: 0.25 }])
    p.reconcile(sync(0), you(0), [cast, { ...viewed, Claims: [{ Seq: 1, Ability: 'bolt', Launch: 0, Target: 9 }] }])
    expect(p.shots(1)).toEqual([])
  })
})
