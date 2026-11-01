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

  test('drops the states before a tick', () => {
    const tl = timeline(1, 2, 3, 4)
    tl.dropBefore(3)
    expect(tl.size).toBe(2)
    expect(tl.sample(4)).toEqual({ from: 's3', to: 's4', t: 1 })
  })

  test('samples a view from the states at its tick and the tick before: a whole tick is its end', () => {
    const tl = timeline(10, 11, 12)
    expect(tl.sample(10.5)).toEqual({ from: 's10', to: 's11', t: 0.5 })
    expect(tl.sample(11)).toEqual({ from: 's10', to: 's11', t: 1 })
    expect(tl.sample(11.25)).toEqual({ from: 's11', to: 's12', t: 0.25 })
    expect(tl.sample(10.5)).toEqual({ from: 's10', to: 's11', t: 0.5 }) // older states are kept
  })

  test('samples nothing without both states', () => {
    const tl = timeline(10, 11, 13)
    expect(tl.sample(10)).toBeUndefined() // needs 9
    expect(tl.sample(9.5)).toBeUndefined()
    expect(tl.sample(12)).toBeUndefined() // needs 12
    expect(tl.sample(12.5)).toBeUndefined() // needs 12
    expect(tl.sample(13.5)).toBeUndefined() // needs 14
    expect(timeline(3).sample(3)).toBeUndefined()
  })

  test('samples nothing when empty or cleared', () => {
    const tl = timeline(1, 2)
    tl.clear()
    expect(tl.size).toBe(0)
    expect(tl.sample(1.5)).toBeUndefined()
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

  test.each([20, 60])(
    'keeps the 0.35 s a view can reach back at %i Hz, behind the newest tick every timeline has',
    (rate) => {
      const timelines = new Timelines()
      timelines.tickSeconds = 1 / rate
      const a = timelines.create<string>()
      const b = timelines.create<string>()
      const keep = Math.ceil(0.35 * rate) // MaxDrift + InterpolationDelay
      for (let tick = 1; tick <= 3 * rate; tick++) timelines.add(a, BigInt(tick), `a${tick}`)
      expect(a.size).toBe(3 * rate) // b has nothing yet, so no view is drawn and nothing drops
      for (let tick = 1; tick <= 2 * rate; tick++) timelines.add(b, BigInt(tick), `b${tick}`)
      // The render clock draws behind b's newest: a keeps those states, though it is a second ahead.
      const newest = 2 * rate
      const oldestView = newest - keep
      expect(a.sample(oldestView)?.from).toBe(`a${oldestView - 1}`)
      expect(b.sample(oldestView)?.from).toBe(`b${oldestView - 1}`)
      expect(a.sample(oldestView - 1)).toBeUndefined()
      expect(b.size).toBe(keep + 2)
    },
  )

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
    expect(timelines.renderTick(0, 0.05)).toBeCloseTo(98)
    timelines.reset()
    expect(tl.size).toBe(0)
    timelines.add(tl, 102n, 's102')
    // A fresh clock starts at its target, tick 100, instead of easing on from tick 98.
    expect(timelines.renderTick(0.016, 0.05)).toBeCloseTo(100)
  })

  test('draws the newest tick every timeline has, minus the interpolation delay', () => {
    const timelines = new Timelines()
    const a = timelines.create<string>()
    const b = timelines.create<string>()
    expect(timelines.renderTick(0.016, 0.05)).toBeUndefined()
    for (let tick = 95n; tick <= 100n; tick++) timelines.add(a, tick, `a${tick}`)
    expect(timelines.renderTick(0.016, 0.05)).toBeUndefined() // b has none
    for (let tick = 95n; tick <= 99n; tick++) timelines.add(b, tick, `b${tick}`)
    expect(timelines.newestTick).toBe(99)
    expect(timelines.renderTick(0.016, 0)).toBeUndefined()
    // 99 ticks * 0.05 s - 0.1 s is tick 97.
    expect(timelines.renderTick(0.016, 0.05)).toBeCloseTo(97)
  })

  test('waits at the newest tick while no state comes, never stepping back, then jumps ahead', () => {
    const timelines = new Timelines()
    const tl = timelines.create<string>()
    for (let tick = 90n; tick <= 100n; tick++) timelines.add(tl, tick, `s${tick}`)
    const drawn = [timelines.renderTick(0, 0.05)!]
    for (let i = 0; i < 60; i++) drawn.push(timelines.renderTick(1 / 60, 0.05)!) // a second with no state
    expect(drawn.at(-1)).toBe(100)
    expect(drawn.every((tick, i) => i === 0 || tick >= drawn[i - 1]!)).toBe(true)
    for (let tick = 101n; tick <= 120n; tick++) timelines.add(tl, tick, `s${tick}`)
    expect(timelines.renderTick(1 / 60, 0.05)).toBeCloseTo(118) // a second behind its target
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
    expect(stepRenderTime(NaN, 4.9, 5, 0.016)).toBe(4.9)
  })

  test('advances by dt, then eases toward the target by time', () => {
    expect(stepRenderTime(1, 1.1, 2, 0.02)).toBeCloseTo(1.02 + 0.08 * (1 - Math.exp(-0.04)), 12)
    expect(stepRenderTime(1, 0.98, 2, 0.02)).toBeCloseTo(1.02 - 0.04 * (1 - Math.exp(-0.04)), 12)
  })

  test('runs the same at any frame rate', () => {
    const run = (fps: number) => {
      let at = 1
      for (let i = 0; i < fps / 10; i++) at = stepRenderTime(at, 1.2, 2, 1 / fps) // 0.1 s, its target ahead
      return at
    }
    expect(Math.abs(run(240) - run(60))).toBeLessThan(0.002)
  })

  test('jumps ahead to the target when more than 0.25 s behind', () => {
    expect(stepRenderTime(1, 2, 3, 0.016)).toBe(2)
  })

  test('never steps back', () => {
    expect(stepRenderTime(1, 0.5, 2, 0.016)).toBe(1)
    expect(stepRenderTime(1, 0.9, 2, 10)).toBe(1)
  })

  test('never passes newest', () => {
    expect(stepRenderTime(1, 1.1, 1.05, 0.2)).toBe(1.05)
    expect(stepRenderTime(1, 2, 1.5, 0.016)).toBe(1.5)
  })
})
