import { describe, expect, test } from 'vitest'
import type { BodyState, BodySync, ShotState } from './body_sync_event'
import type { InputFrame } from './input_command'
import { Prediction } from './prediction'
import type { EndedShot, PredictedEcho, PredictedShot, Predictor } from './predictor'

const tickSeconds = 0.05
const fadeSeconds = 0.1
const keep = Math.exp(-tickSeconds / fadeSeconds) // what a tick leaves of a correction


/**
 * A stand-in for the wasm: each frame moves the Body by its move, and button 0 casts a shot that flies
 * up 1 a tick. A frame with a view ends every shot in flight half way up its tick, on Body 2 there.
 * Your Echo, if the sync has one, trails your Body a tick.
 */
function predictor(): Predictor {
  let at = { x: 0, y: 0 }
  let tick = 0
  let shots: PredictedShot[] = []
  let echo: PredictedEcho | undefined
  return {
    version: 'this build',
    interpolationSeconds: 0.1,
    reset: (sync, you) => {
      at = { x: you.X, y: you.Y }
      tick = Number(sync.Tick)
      shots = sync.You.Shots.map(({ Type, Seq, Ability, Launch, X, Y, PrevX, PrevY, Radius }) => ({
        Type, Seq, Ability, Launch, X, Y, PrevX, PrevY, Radius,
      }))
      const e = sync.You.EchoPath[0]
      echo = e && { ...e, PrevX: e.X, PrevY: e.Y }
    },
    seeTargets: () => {},
    forget: () => {},
    peek: (x, y) => ({ x: at.x + x, y: at.y + y }),
    step: (f) => {
      tick++
      if (echo) echo = { X: at.x, Y: at.y, PrevX: echo.X, PrevY: echo.Y }
      at = { x: at.x + f.MoveX, y: at.y + f.MoveY }
      const end = ({ Type, Seq, Ability, Launch, X, Y, Radius }: PredictedShot): EndedShot => ({
        ...{ Type, Seq, Ability, Launch, X, Y: Y + 0.5, PrevX: X, PrevY: Y, Radius, Until: 0.5 },
        ...{ OnTarget: true, TargetSerial: 2n, OffsetX: 0, OffsetY: -0.75 },
      })
      const ended = f.ViewTick > 0 ? shots.map(end) : []
      if (f.ViewTick > 0) shots = []
      if (f.Buttons & 1) {
        const key = { Seq: f.Seq, Ability: 'bolt', Launch: 0 }
        shots.push({ Type: 'bolt', ...key, X: at.x, Y: at.y, PrevX: at.x, PrevY: at.y, Radius: 0.25 })
      }
      shots = shots.map((s) => ({ ...s, PrevX: s.X, PrevY: s.Y, Y: s.Y + 1 }))
      return { at, ended }
    },
    shots: () => [...shots],
    echo: () => echo,
    kit: () => ({ Tick: tick, Slots: [{ Ability: 'bolt', ReadyTick: 0 }] }),
  }
}

const you = (x: number): BodyState => ({
  EntityID: 1,
  Serial: 1n,
  PlayerID: 1,
  Nickname: 'P1',
  Champion: 'starter',
  Team: 0,
  X: x,
  Y: 0,
  Radius: 0.5,
  Tags: 0n,
  Effects: [],
})
const sync = (x: number): BodySync => ({
  Tick: 100n,
  TickSeconds: tickSeconds,
  SimVersion: 'this build',
  You: {
    Body: 1,
    InputSession: 7,
    InputAck: 0,
    InputQueued: 1,
    InputCap: 10,
    MoveSpeed: 5,
    SnapSeconds: 0.6, // 3 units
    FadeSeconds: fadeSeconds,
    Champion: 'starter',
    Cooldowns: [],
    Effects: [],
    EffectsNext: 0,
    Shots: [],
    EchoDelay: 0,
    EchoPath: [],
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

  test('the snap line is seconds of your move speed: a faster champion fades a longer correction', () => {
    const p = ahead()
    const fast = sync(6)
    fast.You.MoveSpeed = 20 // a 12-unit line
    p.reconcile(fast, you(6), [])
    expect(p.drawn(0)).toEqual({ x: 1, y: 0 })
  })

  test('starts over after clear', () => {
    const p = ahead()
    p.step(move(0, 1), tickSeconds)
    p.step({ ...move(0), ViewTick: 97.5 }, tickSeconds) // ends the shot
    p.clear()
    expect(p.position).toBeUndefined()
    expect(() => p.step(move(1), tickSeconds)).toThrow()
    expect(() => p.peek(1, 0)).toThrow()
    expect(() => p.drawn(0)).toThrow()
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
      {
        ...{ Type: 'bolt', Ability: 'bolt', Seq: 3, Launch: 0 },
        ...{ PrevX: 2, PrevY: 2, X: 2, Y: 3, VelX: 0, VelY: 1, Radius: 0.25, Remaining: 4 },
      },
    ]
    p.reconcile(s, you(2), [right(4)])
    expect(p.shots(1)).toEqual([{ Type: 'bolt', X: 2, Y: 4, Radius: 0.25 }])
  })

  test('a correction keeps your shots where they are drawn, then they fade with your Body to where they end', () => {
    const p = new Prediction(predictor())
    p.reconcile(sync(0), you(0), [])
    const frames = [right(1), move(0, 1, 2)] // walk to x = 1, then cast there
    for (const f of frames) p.step(f, tickSeconds)
    p.reconcile(sync(0.5), you(0.5), frames) // the server's Body is half a unit ahead, so its shot left from 1.5
    expect(p.shots(1)).toEqual([{ Type: 'bolt', X: 1, Y: 1, Radius: 0.25 }])
    p.step(move(0, 0, 3), tickSeconds)
    expect(p.shots(0)).toEqual([{ Type: 'bolt', X: 1, Y: 1, Radius: 0.25 }])
    expect(p.shots(1)[0]?.X).toBeCloseTo(1.5 - 0.5 * keep)
    expect(p.drawn(1).x).toBeCloseTo(1.5 - 0.5 * keep)
    p.step({ ...move(0, 0, 4), ViewTick: 97.5 }, tickSeconds) // it ends at y = 2.5
    expect(p.shots(0)[0]?.X).toBeCloseTo(1.5 - 0.5 * keep)
    expect(p.shots(0.5)).toEqual([{ Type: 'bolt', X: 1.5, Y: 2.5, Radius: 0.25 }])
  })

  test('a shot cast while your Body fades leaves where your Body is drawn', () => {
    const p = ahead()
    p.reconcile(sync(0.5), you(0.5), [right(2)]) // the server is half a unit behind
    p.step(move(0, 1, 3), tickSeconds) // casts at x = 1.5
    expect(p.shots(0)[0]?.X).toBeCloseTo(1.5 + 0.5 * keep)
    expect(p.shots(0)[0]?.X).toBeCloseTo(p.drawn(1).x)
  })

  test('a shot a correction moves far jumps', () => {
    const p = new Prediction(predictor())
    p.reconcile(sync(0), you(0), [])
    const cast = move(0, 1, 1)
    p.step(cast, tickSeconds)
    p.reconcile(sync(5), you(5), [cast]) // the server's Body, and so its shot, are 5 units over
    expect(p.shots(0.5)).toEqual([{ Type: 'bolt', X: 5, Y: 0.5, Radius: 0.25 }])
  })

  test('a replay ends your shot as the step did, and a shot the server flew on shows', () => {
    const p = new Prediction(predictor())
    p.reconcile(sync(0), you(0), [])
    const cast = move(0, 1, 1)
    p.step(cast, tickSeconds)
    const hit: InputFrame = { ...move(0, 0, 2), ViewTick: 97.5 }
    p.step(hit, tickSeconds)
    expect(p.shots(1)).toEqual([])
    p.reconcile(sync(0), you(0), [cast, hit]) // the server has applied neither frame
    expect(p.shots(1)).toEqual([])
    const flying = sync(0)
    const shot: ShotState = {
      ...{ Type: 'bolt', Ability: 'bolt', Seq: 1, Launch: 0 },
      ...{ PrevX: 0, PrevY: 1, X: 0, Y: 2, VelX: 0, VelY: 1, Radius: 0.25, Remaining: 4 },
    }
    flying.You.Shots = [shot]
    p.reconcile(flying, you(0), []) // it applied both, and its shot met nothing
    expect(p.shots(1)).toEqual([{ Type: 'bolt', X: 0, Y: 2, Radius: 0.25 }])
    expect(p.shots(0.5)).toEqual([{ Type: 'bolt', X: 0, Y: 1.5, Radius: 0.25 }]) // flying the server's tick
  })

  test('a shot that ended in the newest tick flies on to where it ended, through body_syncs', () => {
    const p = new Prediction(predictor())
    p.reconcile(sync(0), you(0), [])
    const cast = move(0, 1, 1)
    p.step(cast, tickSeconds) // the shot flies up to y = 1
    const hit: InputFrame = { ...move(0, 0, 2), ViewTick: 97.5 }
    p.step(hit, tickSeconds) // and ends at y = 1.5, half way through the tick
    expect(p.shots(0)).toEqual([{ Type: 'bolt', X: 0, Y: 1, Radius: 0.25 }])
    expect(p.shots(0.25)).toEqual([{ Type: 'bolt', X: 0, Y: 1.25, Radius: 0.25 }])
    expect(p.shots(0.5)).toEqual([{ Type: 'bolt', X: 0, Y: 1.5, Radius: 0.25 }])
    expect(p.shots(0.6)).toEqual([])
    p.reconcile(sync(0), you(0), [cast, hit]) // the replay ends it again
    expect(p.shots(0.25)).toEqual([{ Type: 'bolt', X: 0, Y: 1.25, Radius: 0.25 }])
    p.reconcile(sync(0), you(0), []) // the server applied both: nothing to replay
    expect(p.shots(0.25)).toEqual([{ Type: 'bolt', X: 0, Y: 1.25, Radius: 0.25 }])
    p.step(move(0, 0, 3), tickSeconds)
    expect(p.shots(0)).toEqual([])
  })

  test('a correction keeps where an ending shot is drawn from, and it still ends where it ended', () => {
    const p = new Prediction(predictor())
    p.reconcile(sync(0), you(0), [])
    const cast = move(0, 1, 1)
    const hit: InputFrame = { ...move(0, 0, 2), ViewTick: 97.5 }
    p.step(cast, tickSeconds)
    p.step(hit, tickSeconds) // the shot flies on from y = 1 and ends at y = 1.5
    p.reconcile(sync(0.5), you(0.5), [cast, hit]) // the server's shot left half a unit over
    expect(p.shots(0)).toEqual([{ Type: 'bolt', X: 0, Y: 1, Radius: 0.25 }])
    expect(p.shots(0.5)).toEqual([{ Type: 'bolt', X: 0.5, Y: 1.5, Radius: 0.25 }])
  })

  test('a shot that ended on a Body ends where end says this screen draws it', () => {
    const p = new Prediction(predictor())
    p.reconcile(sync(0), you(0), [])
    p.step(move(0, 1, 1), tickSeconds) // the shot flies up to y = 1
    p.step({ ...move(0, 0, 2), ViewTick: 97.5 }, tickSeconds) // and ends on Body 2, half way through the tick
    const end = (s: EndedShot) => (s.OnTarget && s.TargetSerial === 2n ? { X: 3, Y: 4 } : undefined)
    expect(p.shots(0.25, end)).toEqual([{ Type: 'bolt', X: 1.5, Y: 2.5, Radius: 0.25 }])
    expect(p.shots(0.25)).toEqual([{ Type: 'bolt', X: 0, Y: 1.25, Radius: 0.25 }])
  })

  test('draws your Echo where the frames take it, through the newest tick, and none without one', () => {
    const p = new Prediction(predictor())
    const s = sync(0)
    p.reconcile(s, you(0), [])
    expect(p.echo(0.5)).toBeUndefined()
    s.You.EchoDelay = 20
    s.You.EchoPath = [{ X: -1, Y: 0 }]
    p.reconcile(s, you(0), [right(1), right(2)]) // after the replays, it goes from x = 0 to 1
    expect(p.echo(0)).toEqual({ x: 0, y: 0 })
    expect(p.echo(0.5)).toEqual({ x: 0.5, y: 0 })
    p.step(right(3), tickSeconds)
    expect(p.echo(1)).toEqual({ x: 2, y: 0 })
    p.clear()
    expect(p.echo(0.5)).toBeUndefined()
  })

  test('a correction keeps your Echo where it is drawn, then fades', () => {
    const p = new Prediction(predictor())
    const s = sync(0)
    s.You.EchoDelay = 20
    s.You.EchoPath = [{ X: -1, Y: 0 }]
    p.reconcile(s, you(0), [])
    const frames = [right(1), right(2)]
    for (const f of frames) p.step(f, tickSeconds) // your Echo goes from x = 0 to 1
    p.reconcile(s, you(0.5), frames) // the server's Body is half a unit ahead, and so is your Echo
    expect(p.echo(1)).toEqual({ x: 1, y: 0 })
    p.step(right(3), tickSeconds)
    expect(p.echo(1)?.x).toBeCloseTo(2.5 - 0.5 * keep)
  })
})
