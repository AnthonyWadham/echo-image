import { describe, expect, test } from 'vitest'
import type { InputFrame } from './input_command'
import { InputStream, newSession } from './input_stream'

const tick = 0.25 // exact in binary, so step counts are too
const input = { MoveX: 1, MoveY: 0, AimX: 2, AimY: -1, Buttons: 1 }
const seqs = (frames: readonly InputFrame[]) => frames.map((f) => f.Seq)
const you = (InputSession: number, InputAck: number) => ({
  Body: 1,
  InputSession,
  InputAck,
  MoveSpeed: 5,
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
      Frames: [{ Seq: 1, MoveX: 1, MoveY: -1, AimX: 0, AimY: 0, Buttons: 0 }],
    })
    expect(s.unacked).toEqual([])
  })

  test('before playing, sends only when the move direction changes', () => {
    const s = new InputStream(7)
    s.join({ x: 0, y: 0 })
    expect(s.moveChanged({ x: 0, y: 0 })).toBeUndefined()
    expect(s.moveChanged({ x: 1, y: 0 })?.Frames).toEqual([
      { Seq: 2, MoveX: 1, MoveY: 0, AimX: 0, AimY: 0, Buttons: 0 },
    ])
    expect(s.moveChanged({ x: 1, y: 0 })).toBeUndefined()
    expect(s.moveChanged({ x: 0, y: 0 })?.Frames[0]?.Seq).toBe(3)
  })

  test('steps once per tick of elapsed time', () => {
    const s = new InputStream(7)
    expect(s.step(0.125, tick, input)).toEqual([])
    const sends = s.step(0.125, tick, input)
    expect(sends).toEqual([{ Session: 7, Frames: [{ Seq: 1, ...input }] }])
    expect(seqs(s.unacked)).toEqual([1])
  })

  test('steps at most 3 times per animation frame', () => {
    const s = new InputStream(7)
    expect(s.step(10, tick, input)).toHaveLength(3)
    expect(s.step(0, tick, input)).toEqual([])
  })

  test('never steps without a tick length', () => {
    expect(new InputStream(7).step(1, 0, input)).toEqual([])
  })

  test('sends the newest 8 unacked frames with each step', () => {
    const s = new InputStream(7)
    for (let i = 0; i < 9; i++) s.step(tick, tick, input)
    const [send] = s.step(tick, tick, input)
    expect(seqs(send?.Frames ?? [])).toEqual([3, 4, 5, 6, 7, 8, 9, 10])
  })

  test('keeps at most 128 unacked frames', () => {
    const s = new InputStream(7)
    for (let i = 0; i < 130; i++) s.step(tick, tick, input)
    expect(s.unacked).toHaveLength(128)
    expect(s.unacked[0]?.Seq).toBe(3)
  })

  test('ack drops the frames its own session applied', () => {
    const s = new InputStream(7)
    for (let i = 0; i < 5; i++) s.step(tick, tick, input)
    s.ack(you(9, 4))
    expect(seqs(s.unacked)).toEqual([1, 2, 3, 4, 5])
    s.ack(you(7, 3))
    expect(seqs(s.unacked)).toEqual([4, 5])
  })

  test('reset forgets unacked frames and step time; Seq carries on', () => {
    const s = new InputStream(7)
    s.join({ x: 0, y: 0 })
    s.step(tick * 1.5, tick, input)
    s.reset()
    expect(s.unacked).toEqual([])
    expect(s.step(tick * 0.75, tick, input)).toEqual([])
    expect(s.join({ x: 0, y: 0 }).Frames[0]?.Seq).toBe(3)
  })
})
