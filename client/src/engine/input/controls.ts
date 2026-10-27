import type { Vec2 } from '../math/vec2'

/** WASD and arrow keys (KeyboardEvent.code) as a move direction, not normalized. */
export function moveFrom(keys: ReadonlySet<string>): Vec2 {
  const held = (...codes: string[]) => (codes.some((code) => keys.has(code)) ? 1 : 0)
  return {
    x: held('KeyD', 'ArrowRight') - held('KeyA', 'ArrowLeft'),
    y: held('KeyW', 'ArrowUp') - held('KeyS', 'ArrowDown'),
  }
}

/**
 * InputFrame.Buttons from what's held: bit N casts kit slot N. The left mouse button is slot 0, Space
 * slot 1.
 */
export function buttonsFrom(keys: ReadonlySet<string>, mouseButtons: number): number {
  return (mouseButtons & 1) | (keys.has('Space') ? 1 << 1 : 0)
}

/** Keyboard and mouse state from DOM events. All of it is released when the window loses focus. */
export class Controls {
  readonly #keys = new Set<string>()
  #pointer: Vec2 | undefined
  #mouseButtons = 0 // PointerEvent.buttons

  constructor(canvas: HTMLElement) {
    window.addEventListener('keydown', (e) => this.#keys.add(e.code))
    window.addEventListener('keyup', (e) => this.#keys.delete(e.code))
    window.addEventListener('blur', () => {
      this.#keys.clear()
      this.#mouseButtons = 0
    })
    const onPointer = (e: PointerEvent) => {
      this.#pointer = { x: e.clientX, y: e.clientY }
      this.#mouseButtons = e.buttons
    }
    canvas.addEventListener('pointerdown', (e) => {
      canvas.setPointerCapture(e.pointerId) // so the release counts outside the window too
      onPointer(e)
    })
    window.addEventListener('pointermove', onPointer)
    window.addEventListener('pointerup', onPointer)
    canvas.addEventListener('contextmenu', (e) => e.preventDefault())
  }

  get move(): Vec2 {
    return moveFrom(this.#keys)
  }

  /** The mouse in client pixels, once it has moved over the page. */
  get pointer(): Vec2 | undefined {
    return this.#pointer
  }

  /** InputFrame.Buttons: see buttonsFrom. */
  get buttons(): number {
    return buttonsFrom(this.#keys, this.#mouseButtons)
  }
}
