import { afterEach, describe, expect, test, vi } from 'vitest'
import { Controls, buttonsFrom, moveFrom } from './controls'

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

describe('Controls', () => {
  afterEach(() => vi.unstubAllGlobals())

  test('each frame takes the buttons held and those pressed since a frame last took them', () => {
    const win = new EventTarget()
    vi.stubGlobal('window', win)
    const canvas = Object.assign(new EventTarget(), { setPointerCapture: () => {} })
    const controls = new Controls(canvas as unknown as HTMLElement)
    const send = (to: EventTarget, type: string, event: object) =>
      to.dispatchEvent(Object.assign(new Event(type), event))
    send(win, 'keydown', { code: 'Space' })
    send(win, 'keyup', { code: 'Space' })
    expect(controls.takeButtons()).toBe(2) // a tap between two frames
    expect(controls.takeButtons()).toBe(0) // goes in one
    const pointer = { pointerId: 1, clientX: 0, clientY: 0 }
    send(canvas, 'pointerdown', { ...pointer, buttons: 1 })
    send(win, 'pointerup', { ...pointer, buttons: 0 })
    expect(controls.takeButtons()).toBe(1) // a click too
    send(canvas, 'pointerdown', { ...pointer, buttons: 1 })
    expect(controls.takeButtons()).toBe(1)
    expect(controls.takeButtons()).toBe(1) // still held
    send(win, 'pointerup', { ...pointer, buttons: 0 })
    expect(controls.takeButtons()).toBe(0)
  })
})
