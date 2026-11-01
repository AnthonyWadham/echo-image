import { describe, expect, test } from 'vitest'
import { Timelines } from '../../engine/sync/timeline'
import type { BodySync } from './body_sync_event'
import { LocalPlayer } from './local_player'
import type { Predictor } from './predictor'

const session = 7
const still = { move: { x: 0, y: 0 }, ground: undefined, buttons: 0, view: undefined }

/**
 * A stand-in for the wasm: each frame moves the Body by its move. A new frame with a view claims a
 * shot of its own on Body 2. seen logs the syncs it was given, and forget.
 */
function predictor(seen: string[] = []): Predictor {
  let at = { x: 0, y: 0 }
  return {
    reset: (_, you) => {
      at = { x: you.X, y: you.Y }
    },
    seeBodies: (s) => seen.push(`bodies ${s.Tick}`),
    seeEchoes: (s) => seen.push(`echoes ${s.Tick}`),
    forget: () => seen.push('forget'),
    peek: (x, y) => ({ x: at.x + x, y: at.y + y }),
    step: (f) => {
      at = { x: at.x + f.MoveX, y: at.y + f.MoveY }
      return { at, claims: f.ViewTick > 0 ? [{ Seq: f.Seq, Ability: 'bolt', Launch: 0, Target: 2 }] : [], ended: [] }
    },
    replay: (f) => {
      at = { x: at.x + f.MoveX, y: at.y + f.MoveY }
      return at
    },
    shots: () => [],
    kit: () => ({ Tick: 0, Slots: [] }),
  }
}
const newPlayer = (timelines = new Timelines(), seen: string[] = []) =>
  new LocalPlayer(timelines, session, predictor(seen))

/** A body_sync at tick for you as Body 1, by default alone at (tick, 0). */
const bodySync = (tick: number, ack = 0, bodies = [{ EntityID: 1, X: tick, Y: 0 }]): BodySync => ({
  Tick: BigInt(tick),
  TickSeconds: 0.05,
  You: {
    Body: 1,
    InputSession: session,
    InputAck: ack,
    MoveSpeed: 5,
    Champion: 'starter',
    Cooldowns: [],
    Effects: [],
    EffectsNext: 0,
    Shots: [],
  },
  Bodies: bodies.map((b) => ({
    ...{ PlayerID: b.EntityID, Nickname: `P${b.EntityID}`, Team: 0, Radius: 0.5, Tags: 0n },
    ...b,
  })),
})

/** Connected and playing Body 1, after the join frame (Seq 1). */
function playing(timelines = new Timelines()): LocalPlayer {
  const player = newPlayer(timelines)
  player.setConnected(true, { x: 0, y: 0 })
  player.onBodySync(bodySync(100))
  return player
}

describe('LocalPlayer', () => {
  test('joins on each connect and starts everything over when the stream drops', () => {
    const timelines = new Timelines()
    const echoes = timelines.create<string>()
    const player = newPlayer(timelines)
    expect(player.setConnected(true, { x: 1, y: 0 })?.Frames).toEqual([
      { Seq: 1, MoveX: 1, MoveY: 0, AimX: 0, AimY: 0, Buttons: 0, ViewTick: 0, Claims: [] },
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
    expect(player.drawnShots).toEqual([])
    expect(player.bodies.size).toBe(0)
    expect(echoes.size).toBe(0)
    expect(player.input.unacked).toEqual([])
    expect(player.setConnected(true, { x: 0, y: 0 })?.Frames[0]?.Seq).toBe(3)
  })

  test('plays once a body_sync has your Body', () => {
    const player = newPlayer()
    player.setConnected(true, { x: 0, y: 0 })
    player.onBodySync(bodySync(100, 0, [{ EntityID: 2, X: 0, Y: 0 }]))
    expect(player.playing).toBe(false)
    player.onBodySync(bodySync(101))
    expect(player.playing).toBe(true)
    expect(player.me?.X).toBe(101)
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
    const held = { move: { x: 1, y: 0 }, ground: { x: 103, y: -2 }, buttons: 1, view: undefined }
    const [input] = player.inputsToSend(0.05, held)
    expect(input).toEqual({
      Session: session,
      Frames: [{ Seq: 2, MoveX: 1, MoveY: 0, AimX: 1, AimY: -2, Buttons: 1, ViewTick: 0, Claims: [] }],
    })
  })

  test('sends the render tick on screen with each frame, and the hits it claims', () => {
    const player = playing()
    const [input] = player.inputsToSend(0.05, { ...still, view: 98.25 })
    expect(input?.Frames.at(-1)).toEqual({
      ...{ Seq: 2, MoveX: 0, MoveY: 0, AimX: 0, AimY: 0, Buttons: 0, ViewTick: 98.25 },
      Claims: [{ Seq: 2, Ability: 'bolt', Launch: 0, Target: 2 }],
    })
    expect(player.input.unacked.at(-1)?.Claims).toHaveLength(1)
  })

  test('gives the predictor every body_sync and echo_sync, and forgets them on leaving the stream', () => {
    const seen: string[] = []
    const player = newPlayer(new Timelines(), seen)
    player.setConnected(true, { x: 0, y: 0 })
    player.onBodySync(bodySync(100, 0, [{ EntityID: 2, X: 0, Y: 0 }])) // not playing yet
    player.onEchoSync({ Tick: 100n, Echoes: [] })
    player.setConnected(false, { x: 0, y: 0 })
    expect(seen).toEqual(['bodies 100', 'echoes 100', 'forget'])
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
    const timelines = new Timelines()
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
