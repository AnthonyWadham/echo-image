import { expect, test } from 'vitest'
import { endOn } from './end_on'

const drawn = (serial: bigint) => (serial === 1n ? { x: 2, y: -1 } : undefined) // this screen draws only serial 1

test('a shot that ended on a Body or Echo ends on it, where this screen draws it', () => {
  expect(endOn({ OnTarget: true, TargetSerial: 1n, OffsetX: -0.5, OffsetY: 0.25 }, drawn)).toEqual({ X: 1.5, Y: -0.75 })
})

test('one that ended on nothing, or on what this screen does not draw, ends where it ended', () => {
  expect(endOn({ OnTarget: false, TargetSerial: 1n, OffsetX: 0, OffsetY: 0 }, drawn)).toBeUndefined()
  expect(endOn({ OnTarget: true, TargetSerial: 7n, OffsetX: 0, OffsetY: 0 }, drawn)).toBeUndefined()
})
