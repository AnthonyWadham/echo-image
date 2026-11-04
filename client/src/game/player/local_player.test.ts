import { describe, expect, test } from 'vitest'
import { Timelines } from '../../engine/sync/timeline'
import { BodyKind, EchoKind } from '../track/target_sync_event'
import type { BodySync } from './body_sync_event'
import { LocalPlayer } from './local_player'
import type { PredictedEcho, Predictor } from './predictor'

const session = 7
const sim = 'this build'
const still = { move: { x: 0, y: 0 }, ground: undefined, takeButtons: () => 0, view: undefined }

/**
 * A stand-in for the wasm: each frame moves the Body by its move, and your Echo stays where the sync had
 * it. seen logs the syncs it was given, and forget.
 */
function predictor(seen: string[] = []): Predictor {
  let at = { x: 0, y: 0 }
  let echo: PredictedEcho | undefined
  return {
    version: sim,
    interpolationSeconds: 0.1,
    agrees: () => {
      seen.push('agrees')
      return false
    },
    reset: (s) => {
      at = { x: s.You.X, y: s.You.Y }
      const e = s.You.EchoPath[0]
      echo = e && { ...e, PrevX: e.X, PrevY: e.Y }
    },
    seeTargets: (s) => seen.push(`targets ${s.Tick}`),
    forget: () => seen.push('forget'),
    peek: (x, y) => ({ x: at.x + x, y: at.y + y }),
    step: (f) => {
      at = { x: at.x + f.MoveX, y: at.y + f.MoveY }
      return { at, ended: [] }
    },
    shots: () => [],
    echo: () => echo,
    kit: () => ({ Tick: 0, Slots: [] }),
  }
}
const newPlayer = (timelines = new Timelines(0.1), seen: string[] = []) =>
  new LocalPlayer(timelines, session, predictor(seen))

/** A body_sync at tick for you as Body 1 at (tick, 0), by default with only your Body's. */
const bodySync = (tick: number, ack = 0, bodies = [1]): BodySync => ({
  Tick: BigInt(tick),
  TickSeconds: 0.05,
  SimVersion: sim,
  You: {
    Body: 1,
    Team: 0,
    X: tick,
    Y: 0,
    Radius: 0.5,
    InputSession: session,
    InputAck: ack,
    InputQueued: 1,
    InputCap: 10,
    InputTarget: 1,
    InputHealth: { Coasts: 0, DroppedFrames: 0, OversizedInputs: 0, FlooredViews: 0 },
    MoveSpeed: 5,
    SnapSeconds: 0.6,
    FadeSeconds: 0.1,
    Champion: 'starter',
    Cooldowns: [],
    Effects: [],
    EffectsNext: 0,
    Shots: [],
    EchoDelay: 0,
    EchoPath: [],
  },
  Bodies: bodies.map((id) => ({
    ...{ EntityID: id, Serial: BigInt(id), PlayerID: id, Nickname: `P${id}`, Champion: 'starter', Effects: [] },
  })),
})

/** Connected and playing Body 1, after the join frame (Seq 1). */
function playing(timelines = new Timelines(0.1)): LocalPlayer {
  const player = newPlayer(timelines)
  player.setConnected(true, { x: 0, y: 0 })
  player.onBodySync(bodySync(100))
  return player
}

describe('LocalPlayer', () => {
  test('joins on each connect and starts everything over when the stream drops', () => {
    const timelines = new Timelines(0.1)
    const echoes = timelines.create<string>()
    const player = newPlayer(timelines)
    expect(player.setConnected(true, { x: 1, y: 0 })?.Frames).toEqual([
      { Seq: 1, MoveX: 1, MoveY: 0, AimX: 0, AimY: 0, Buttons: 0, ViewTick: 0 },
    ])
    expect(player.setConnected(true, { x: 1, y: 0 })).toBeUndefined()
    player.onBodySync(bodySync(100))
    timelines.add(echoes, 100n, 'e100')
    player.inputsToSend(0.05, still)
    expect(player.playing).toBe(true)

    expect(player.kit).toBeDefined()
    expect(player.setConnected(false, { x: 0, y: 0 })).toBeUndefined()
    expect(player.playing).toBe(false)
    expect(player.kit).toBeUndefined()
    expect(player.drawnShots()).toEqual([])
    expect(player.bodies.size).toBe(0)
    expect(echoes.size).toBe(0)
    expect(player.input.unacked).toEqual([])
    expect(player.setConnected(true, { x: 0, y: 0 })?.Frames[0]?.Seq).toBe(3)
  })

  test('plays once a body_sync has your Body', () => {
    const player = newPlayer()
    player.setConnected(true, { x: 0, y: 0 })
    player.onBodySync(bodySync(100, 0, [2]))
    expect(player.playing).toBe(false)
    player.onBodySync(bodySync(101))
    expect(player.playing).toBe(true)
    expect(player.prediction.position).toEqual({ x: 101, y: 0 }) // from You
  })

  test("plays only while the shard runs your predictor's sim", () => {
    const player = playing()
    player.onBodySync({ ...bodySync(101), SimVersion: 'another build' })
    expect(player.otherSim).toBe('another build')
    expect(player.playing).toBe(false)
    expect(player.bodies.newestTick).toBe(100) // its Bodies aren't kept
    expect(player.inputsToSend(0.05, { ...still, move: { x: 1, y: 0 } })).toEqual([])
    player.onBodySync(bodySync(102))
    expect(player.otherSim).toBeUndefined()
    expect(player.playing).toBe(true)
    expect(player.inputsToSend(0.05, still)).not.toEqual([])
  })

  test('keeps your team and input health from the newest body_sync', () => {
    const player = playing()
    const s = bodySync(101)
    const health = { Coasts: 2, DroppedFrames: 1, OversizedInputs: 0, FlooredViews: 3 }
    player.onBodySync({ ...s, You: { ...s.You, Team: 1, InputHealth: health } })
    expect(player.team).toBe(1)
    expect(player.inputHealth).toEqual(health)
  })

  test("asks its predictor to agree only with a body_sync of its own session's", () => {
    const seen: string[] = []
    const player = newPlayer(new Timelines(0.1), seen)
    player.setConnected(true, { x: 0, y: 0 })
    player.onBodySync(bodySync(100)) // nothing predicted yet
    const s = bodySync(101)
    player.onBodySync({ ...s, You: { ...s.You, InputSession: session + 1 } })
    expect(seen).toEqual([])
    player.onBodySync(bodySync(102))
    expect(seen).toEqual(['agrees'])
  })

  test('sends nothing while disconnected', () => {
    expect(newPlayer().inputsToSend(1, { ...still, move: { x: 1, y: 0 } })).toEqual([])
  })

  test('before playing, sends a frame only when the move changes', () => {
    const player = newPlayer()
    player.setConnected(true, { x: 0, y: 0 })
    expect(player.inputsToSend(1, still)).toEqual([])
    expect(player.inputsToSend(1, { ...still, move: { x: 0, y: 1 } })).toHaveLength(1)
    expect(player.input.unacked).toEqual([])
  })

  test('while playing, aims from where the frame moves your Body', () => {
    const player = playing()
    player.onBodySync(bodySync(101))
    const held = { move: { x: 1, y: 0 }, ground: { x: 103, y: -2 }, takeButtons: () => 1, view: undefined }
    const [input] = player.inputsToSend(0.05, held)
    expect(input).toEqual({
      Session: session,
      Frames: [{ Seq: 2, MoveX: 1, MoveY: 0, AimX: 1, AimY: -2, Buttons: 1, ViewTick: 0 }],
    })
  })

  test('a press goes in the next frame built, once, and one made before you play casts nothing', () => {
    let pressed = 1 // a tap the controls keep for the next frame
    const take = () => {
      const buttons = pressed
      pressed = 0
      return buttons
    }
    const held = { ...still, takeButtons: take }
    const player = newPlayer()
    player.setConnected(true, { x: 0, y: 0 })
    player.inputsToSend(0.05, held)
    expect(pressed).toBe(0)
    player.onBodySync(bodySync(100))
    pressed = 1
    expect(player.inputsToSend(0.01, held)).toEqual([]) // no frame yet, so it waits
    expect(pressed).toBe(1)
    const inputs = player.inputsToSend(0.1, held) // two frames: the first takes it
    expect(inputs.map((i) => i.Frames.at(-1)?.Buttons)).toEqual([1, 0])
  })

  test('paces its frames by the queue each body_sync reports', () => {
    const player = playing()
    for (let tick = 101; tick < 140; tick++) {
      const s = bodySync(tick)
      player.onBodySync({ ...s, You: { ...s.You, InputQueued: 0 } })
    }
    expect(player.input.rate).toBeGreaterThan(1)
  })

  test('sends the render tick on screen with each frame', () => {
    const player = playing()
    const [input] = player.inputsToSend(0.05, { ...still, view: 98.25 })
    expect(input?.Frames.at(-1)).toEqual({ Seq: 2, MoveX: 0, MoveY: 0, AimX: 0, AimY: 0, Buttons: 0, ViewTick: 98.25 })
  })

  test('draws your Echo where your predictor has it, until you leave the stream', () => {
    const player = newPlayer()
    player.setConnected(true, { x: 0, y: 0 })
    const s = bodySync(100)
    s.You.EchoDelay = 20
    s.You.EchoPath = [{ X: 80, Y: 0 }]
    player.onBodySync(s)
    expect(player.drawnEcho).toEqual({ x: 80, y: 0 })
    player.setConnected(false, { x: 0, y: 0 })
    expect(player.drawnEcho).toBeUndefined()
  })

  test('gives the predictor each target_sync while the shard runs your sim, and forgets them on leaving', () => {
    const seen: string[] = []
    const timelines = new Timelines(0.1)
    const player = newPlayer(timelines, seen)
    player.setConnected(true, { x: 0, y: 0 })
    player.onTargetSync({ Tick: 99n, Targets: [] }) // before a body_sync says which sim
    player.onBodySync(bodySync(100, 0, [2])) // not playing yet
    expect(timelines.newestTick).toBe(99) // the screen waits for tick 100's target_sync
    const row = { EntityID: 2, Serial: 2n, Owner: 2, Team: 1, X: 0, Y: 0, Radius: 0.5, Tags: 0n, Until: 1 }
    const body = { ...row, Kind: BodyKind }
    const echo = { ...row, EntityID: 3, Serial: 3n, Kind: EchoKind }
    player.onTargetSync({ Tick: 100n, Targets: [echo, body] })
    expect(timelines.newestTick).toBe(100)
    expect([player.bodyRows.newest, player.echoRows.newest]).toEqual([[body], [echo]])
    player.onBodySync({ ...bodySync(101), SimVersion: 'another build' })
    player.onTargetSync({ Tick: 101n, Targets: [] })
    player.setConnected(false, { x: 0, y: 0 })
    expect(seen).toEqual(['targets 100', 'forget'])
  })

  test('predicts your Body from each body_sync plus the frames it has not applied', () => {
    const player = playing()
    const right = { ...still, move: { x: 1, y: 0 } }
    player.inputsToSend(0.05, right)
    player.inputsToSend(0.05, right)
    expect(player.prediction.position).toEqual({ x: 102, y: 0 })
    player.onBodySync(bodySync(101, 2)) // frame 2 applied, frame 3 not
    expect(player.prediction.position).toEqual({ x: 102, y: 0 })
    expect(player.prediction.corrections).toBe(0)
    expect(player.drawn).toEqual({ x: 101, y: 0 })
  })

  test('drops the frames each body_sync acks', () => {
    const player = playing()
    player.inputsToSend(0.05, still)
    player.inputsToSend(0.05, still)
    player.onBodySync(bodySync(101, 2))
    expect(player.input.unacked.map((f) => f.Seq)).toEqual([3])
  })

  test('a tick going backwards in any sync event starts over', () => {
    const timelines = new Timelines(0.1)
    const echoes = timelines.create<string>()
    const player = playing(timelines)
    timelines.add(echoes, 100n, 'e100')
    player.inputsToSend(0.05, still)
    timelines.add(echoes, 5n, 'e5')
    expect(player.bodies.size).toBe(0)
    expect(echoes.newestTick).toBe(5)
    expect(player.playing).toBe(false)
    expect(player.input.unacked).toEqual([])
  })
})
