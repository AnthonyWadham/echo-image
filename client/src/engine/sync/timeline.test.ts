import { describe, expect, test, vi } from 'vitest'
import { Timeline, Timelines, stepRenderTime, toTick } from './timeline'

const timeline = (...ticks: number[]) => {
  const tl = new Timeline<string>()
  for (const tick of ticks) tl.add(tick, `s${tick}`)
  return tl
}

describe('Timeline', () => {
  test('keeps states in tick order and reports the newest', () => {
    const tl = timeline(1, 2, 4)
    expect(tl.size).toBe(3)
    expect(tl.newest).toBe('s4')
    expect(tl.newestTick).toBe(4)
  })

  test('rejects a tick going backwards', () => {
    const tl = timeline(5)
    expect(tl.add(4, 's4')).toBe(false)
    expect(tl.newest).toBe('s5')
    expect(tl.size).toBe(1)
  })

  test('ignores a repeated tick', () => {
    const tl = timeline(5)
    expect(tl.add(5, 'again')).toBe(true)
    expect(tl.newest).toBe('s5')
    expect(tl.size).toBe(1)
  })

  test('keeps the newest 64 states', () => {
    const tl = timeline(...Array.from({ length: 70 }, (_, i) => i + 1))
    expect(tl.size).toBe(64)
    expect(tl.sample(0)?.from).toBe('s7')
  })

  test('samples the two states around a tick', () => {
    expect(timeline(10, 12).sample(11)).toEqual({ from: 's10', to: 's12', t: 0.5 })
  })

  test('drops states older than both sides of the sample', () => {
    const tl = timeline(10, 11, 12, 13)
    expect(tl.sample(12.25)).toEqual({ from: 's12', to: 's13', t: 0.25 })
    expect(tl.size).toBe(2)
  })

  test('clamps t before the first state and past the newest, keeping two', () => {
    expect(timeline(10, 11).sample(9)).toEqual({ from: 's10', to: 's11', t: 0 })
    const tl = timeline(10, 11, 12)
    expect(tl.sample(20)).toEqual({ from: 's11', to: 's12', t: 1 })
    expect(tl.size).toBe(2)
  })

  test('samples a single state as both sides', () => {
    expect(timeline(3).sample(1)).toEqual({ from: 's3', to: 's3', t: 1 })
  })

  test('samples nothing when empty or cleared', () => {
    const tl = timeline(1, 2)
    tl.clear()
    expect(tl.size).toBe(0)
    expect(tl.sample(1)).toBeUndefined()
    expect(tl.newestTick).toBe(0)
  })
})

describe('Timelines', () => {
  test('adds at a uint64 tick', () => {
    const timelines = new Timelines()
    const tl = timelines.create<string>()
    timelines.add(tl, 7n, 's7')
    expect(tl.newest).toBe('s7')
    expect(tl.newestTick).toBe(7)
  })

  test('a tick going backwards in any timeline starts them all over', () => {
    const timelines = new Timelines()
    const onReset = vi.fn()
    timelines.onReset(onReset)
    const a = timelines.create<string>()
    const b = timelines.create<string>()
    timelines.add(a, 100n, 'a100')
    timelines.add(b, 100n, 'b100')
    timelines.add(b, 5n, 'b5')
    expect(a.size).toBe(0)
    expect(b.newest).toBe('b5')
    expect(b.size).toBe(1)
    expect(onReset).toHaveBeenCalledOnce()
  })

  test('reset clears every timeline and the render clock', () => {
    const timelines = new Timelines()
    const tl = timelines.create<string>()
    timelines.add(tl, 100n, 's100')
    expect(timelines.renderTick(0, tl, 0.05)).toBeCloseTo(98)
    timelines.reset()
    expect(tl.size).toBe(0)
    timelines.add(tl, 102n, 's102')
    // A fresh clock starts at its target, tick 100, instead of easing on from tick 98.
    expect(timelines.renderTick(0.016, tl, 0.05)).toBeCloseTo(100)
  })

  test('draws the newest tick minus the interpolation delay', () => {
    const timelines = new Timelines()
    const tl = timelines.create<string>()
    expect(timelines.renderTick(0.016, tl, 0.05)).toBeUndefined()
    for (let tick = 95n; tick <= 100n; tick++) timelines.add(tl, tick, `s${tick}`)
    expect(timelines.renderTick(0.016, tl, 0)).toBeUndefined()
    // 100 ticks * 0.05 s - 0.1 s is tick 98.
    expect(timelines.renderTick(0.016, tl, 0.05)).toBeCloseTo(98)
  })
})

describe('toTick', () => {
  test('converts a uint64 tick', () => {
    expect(toTick(123n)).toBe(123)
    expect(toTick(BigInt(Number.MAX_SAFE_INTEGER))).toBe(Number.MAX_SAFE_INTEGER)
  })

  test('throws past Number.MAX_SAFE_INTEGER', () => {
    expect(() => toTick(BigInt(Number.MAX_SAFE_INTEGER) + 1n)).toThrow(RangeError)
    expect(() => toTick(18446744073709551615n)).toThrow(RangeError)
  })
})

describe('stepRenderTime', () => {
  test('starts at the target', () => {
    expect(stepRenderTime(NaN, 4.9, 0.016)).toBe(4.9)
  })

  test('advances by dt, then eases 5% toward the target', () => {
    expect(stepRenderTime(1, 1.1, 0.02)).toBeCloseTo(1.02 + 0.08 * 0.05, 12)
    expect(stepRenderTime(1, 0.9, 0.02)).toBeCloseTo(1.02 - 0.12 * 0.05, 12)
  })

  test('jumps to the target when more than 0.25 s off', () => {
    expect(stepRenderTime(1, 2, 0.016)).toBe(2)
    expect(stepRenderTime(1, 0.5, 0.016)).toBe(0.5)
    expect(stepRenderTime(1, 1, 10)).toBe(1)
  })
})
