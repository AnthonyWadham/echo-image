import { describe, expect, test } from 'vitest'
import { buttonsFrom, moveFrom } from './controls'

describe('moveFrom', () => {
  test('reads WASD and arrows; opposite keys cancel', () => {
    expect(moveFrom(new Set())).toEqual({ x: 0, y: 0 })
    expect(moveFrom(new Set(['KeyW', 'KeyD']))).toEqual({ x: 1, y: 1 })
    expect(moveFrom(new Set(['ArrowDown', 'ArrowLeft']))).toEqual({ x: -1, y: -1 })
    expect(moveFrom(new Set(['KeyA', 'ArrowRight', 'KeyW']))).toEqual({ x: 0, y: 1 })
  })
})

describe('buttonsFrom', () => {
  test('casts kit slot 0 with the left mouse button and slot 1 with Space', () => {
    expect(buttonsFrom(new Set(), 0)).toBe(0)
    expect(buttonsFrom(new Set(), 1)).toBe(1)
    expect(buttonsFrom(new Set(), 2)).toBe(0) // the right button casts nothing
    expect(buttonsFrom(new Set(['Space']), 0)).toBe(2)
    expect(buttonsFrom(new Set(['Space', 'KeyW']), 1)).toBe(3)
  })
})
