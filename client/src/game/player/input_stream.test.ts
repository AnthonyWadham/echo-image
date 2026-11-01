import { describe, expect, test } from 'vitest'
import type { InputFrame } from './input_command'
import { InputStream, newSession } from './input_stream'

const tick = 0.25 // exact in binary, so step counts are too
const input = { MoveX: 1, MoveY: 0, AimX: 2, AimY: -1, Buttons: 1, ViewTick: 0 }
const next = () => input
const seqs = (frames: readonly InputFrame[]) => frames.map((f) => f.Seq)
const seqsTo = (newest: number, n: number) => Array.from({ length: n }, (_, i) => newest - n + 1 + i)
const you = (InputSession: number, InputAck: number, InputQueued = 1, InputCap = 10) => ({
  Body: 1,
  InputSession,
  InputAck,
  InputQueued,
  InputCap,
  MoveSpeed: 5,
  Champion: 'starter',
  Cooldowns: [],
  Effects: [],
  EffectsNext: 0,
  Shots: [],
  EchoDelay: 0,
  EchoPath: [],
})

describe('newSession', () => {
  test('is a nonzero uint32', () => {
    for (let i = 0; i < 100; i++) {
      const session = newSession()
      expect(Number.isInteger(session)).toBe(true)
      expect(session).toBeGreaterThan(0)
      expect(session).toBeLessThanOrEqual(0xffffffff)
    }
  })
})

describe('InputStream', () => {
  test('join sends one move-only frame', () => {
    const s = new InputStream(7)
    expect(s.join({ x: 1, y: -1 })).toEqual({
      Session: 7,
      Frames: [{ Seq: 1, MoveX: 1, MoveY: -1, AimX: 0, AimY: 0, Buttons: 0, ViewTick: 0 }],
    })
    expect(s.unacked).toEqual([])
  })

  test('before playing, sends only when the move direction changes', () => {
    const s = new InputStream(7)
    s.join({ x: 0, y: 0 })
    expect(s.moveChanged({ x: 0, y: 0 })).toBeUndefined()
    expect(s.moveChanged({ x: 1, y: 0 })?.Frames).toEqual([
      { Seq: 2, MoveX: 1, MoveY: 0, AimX: 0, AimY: 0, Buttons: 0, ViewTick: 0 },
    ])
    expect(s.moveChanged({ x: 1, y: 0 })).toBeUndefined()
    expect(s.moveChanged({ x: 0, y: 0 })?.Frames[0]?.Seq).toBe(3)
  })

  test('steps once per tick of elapsed time', () => {
    const s = new InputStream(7)
    expect(s.step(0.125, tick, next)).toEqual([])
    expect(s.stepTime).toBe(0.125)
    const sends = s.step(0.125, tick, next)
    expect(sends).toEqual([{ Session: 7, Frames: [{ Seq: 1, ...input }] }])
    expect(seqs(s.unacked)).toEqual([1])
  })

  test.each([
    [20, 3],
    [60, 9],
  ])('at %i Hz, steps at most %i times (0.15 s) per animation frame, asking for each as it goes', (hz, most) => {
    const s = new InputStream(7)
    let asked = 0
    const counted = () => {
      asked++
      return input
    }
    expect(s.step(10, 1 / hz, counted)).toHaveLength(most)
    expect(asked).toBe(most)
    expect(s.step(0, 1 / hz, next)).toEqual([])
  })

  test('never steps without a tick length', () => {
    expect(new InputStream(7).step(1, 0, next)).toEqual([])
  })

  test.each([
    [20, 8],
    [60, 24],
  ])('at %i Hz, sends the newest %i unacked frames (0.4 s) a step, no more than the server queues', (hz, resend) => {
    const tick = 1 / hz
    const s = new InputStream(7)
    s.step(tick, tick, next)
    expect(seqs(s.step(tick, tick, next)[0]?.Frames ?? [])).toEqual([2]) // before the server says
    s.ack(you(7, 0, 1, 2 * resend), tick)
    for (let i = 1; i < resend; i++) s.step(tick, tick, next)
    const newest = resend + 2
    expect(seqs(s.step(tick, tick, next)[0]?.Frames ?? [])).toEqual(seqsTo(newest, resend))
    s.ack(you(9, 0, 1, 3), tick) // another session's ack still says how many it queues
    expect(seqs(s.step(tick, tick, next)[0]?.Frames ?? [])).toEqual(seqsTo(newest + 1, 3))
  })

  test.each([
    [20, 128],
    [60, 384],
  ])('at %i Hz, keeps at most %i unacked frames (6.4 s)', (hz, most) => {
    const s = new InputStream(7)
    for (let i = 0; i < most + 2; i++) s.step(1 / hz, 1 / hz, next)
    expect(s.unacked).toHaveLength(most)
    expect(s.unacked[0]?.Seq).toBe(3)
  })

  test('ack drops the frames its own session applied', () => {
    const s = new InputStream(7)
    for (let i = 0; i < 5; i++) s.step(tick, tick, next)
    s.ack(you(9, 4), tick)
    expect(seqs(s.unacked)).toEqual([1, 2, 3, 4, 5])
    s.ack(you(7, 3), tick)
    expect(seqs(s.unacked)).toEqual([4, 5])
  })

  test.each([
    [20, 1],
    [60, 3],
  ])('at %i Hz, paces frames to keep %i (0.05 s) queued: faster while dry, slower while full', (hz, target) => {
    const tick = 1 / hz
    const sent = (queued: number) => {
      const s = new InputStream(7)
      for (let i = 0; i < 2 * hz; i++) s.ack(you(7, 0, queued), tick) // for 2 s
      let frames = 0
      for (let i = 0; i < 100; i++) frames += s.step(tick, tick, next).length
      return { rate: s.rate, frames }
    }
    const at = sent(target)
    expect(at).toEqual({ rate: 1, frames: 100 })
    const dry = sent(0)
    expect(dry.rate).toBeCloseTo(1.05)
    expect(Math.abs(dry.frames - 105)).toBeLessThanOrEqual(1)
    const full = sent(9 * target) // far above: slowed by at most 10%
    expect(full.rate).toBeCloseTo(0.9)
    expect(Math.abs(full.frames - 90)).toBeLessThanOrEqual(1)
  })

  test('only its own session paces it, and reset forgets the pace', () => {
    const s = new InputStream(7)
    for (let i = 0; i < 40; i++) s.ack(you(9, 0, 0), tick)
    expect(s.rate).toBe(1)
    for (let i = 0; i < 40; i++) s.ack(you(7, 0, 0), tick)
    expect(s.rate).toBeGreaterThan(1)
    s.reset()
    expect(s.rate).toBe(1)
  })

  test('reset forgets unacked frames and step time; Seq carries on', () => {
    const s = new InputStream(7)
    s.join({ x: 0, y: 0 })
    s.step(tick * 1.5, tick, next)
    s.reset()
    expect(s.unacked).toEqual([])
    expect(s.step(tick * 0.75, tick, next)).toEqual([])
    expect(s.join({ x: 0, y: 0 }).Frames[0]?.Seq).toBe(3)
  })
})
