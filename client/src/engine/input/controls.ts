import type { Vec2 } from '../math/vec2'

/** WASD and arrow keys (KeyboardEvent.code) as a move direction, not normalized. */
export function moveFrom(keys: ReadonlySet<string>): Vec2 {
  const held = (...codes: string[]) => (codes.some((code) => keys.has(code)) ? 1 : 0)
  return {
    x: held('KeyD', 'ArrowRight') - held('KeyA', 'ArrowLeft'),
    y: held('KeyW', 'ArrowUp') - held('KeyS', 'ArrowDown'),
  }
}

/** Keyboard and mouse state from DOM events. All of it is released when the window loses focus. */
export class Controls {
  readonly #keys = new Set<string>()
  #pointer: Vec2 | undefined
  #buttons = 0

  constructor(canvas: HTMLElement) {
    window.addEventListener('keydown', (e) => this.#keys.add(e.code))
    window.addEventListener('keyup', (e) => this.#keys.delete(e.code))
    window.addEventListener('blur', () => {
      this.#keys.clear()
      this.#buttons = 0
    })
    const onPointer = (e: PointerEvent) => {
      this.#pointer = { x: e.clientX, y: e.clientY }
      this.#buttons = e.buttons & 1 // the left button only
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

  /** Bit 0 while the left mouse button is held. */
  get buttons(): number {
    return this.#buttons
  }
}
