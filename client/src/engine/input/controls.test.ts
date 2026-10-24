import { describe, expect, test } from 'vitest'
import { moveFrom } from './controls'

describe('moveFrom', () => {
  test('reads WASD and arrows; opposite keys cancel', () => {
    expect(moveFrom(new Set())).toEqual({ x: 0, y: 0 })
    expect(moveFrom(new Set(['KeyW', 'KeyD']))).toEqual({ x: 1, y: 1 })
    expect(moveFrom(new Set(['ArrowDown', 'ArrowLeft']))).toEqual({ x: -1, y: -1 })
    expect(moveFrom(new Set(['KeyA', 'ArrowRight', 'KeyW']))).toEqual({ x: 0, y: 1 })
  })
})
