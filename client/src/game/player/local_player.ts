import type { Vec2 } from '../../engine/math/vec2'
import type { Timeline, Timelines } from '../../engine/sync/timeline'
import type { BodyState, BodySync } from './body_sync_event'
import type { Input } from './input_command'
import { InputStream } from './input_stream'

/** What the player holds this frame. ground is the point on the ground under the mouse. */
export interface Held {
  move: Vec2
  ground: Vec2 | undefined
  /** InputFrame.Buttons: the left mouse button casts kit slot 0. */
  buttons: number
}

/** You: every Body's recent states, which one is yours, and the Input you send. */
export class LocalPlayer {
  readonly bodies: Timeline<BodyState[]>
  readonly input: InputStream
  /** body_sync events received. */
  received = 0
  /** Your Body's EntityID, from the newest body_sync. */
  you = 0
  tickSeconds = 0
  readonly #timelines: Timelines
  #connected = false
  #playing = false

  constructor(timelines: Timelines, session: number) {
    this.#timelines = timelines
    this.bodies = timelines.create<BodyState[]>()
    this.input = new InputStream(session)
    timelines.onReset(() => {
      this.#playing = false
      this.input.reset()
    })
  }

  /** The server has given us a Body: input goes out every tick. */
  get playing(): boolean {
    return this.#playing
  }

  /** Your Body in the newest body_sync. */
  get me(): BodyState | undefined {
    return this.bodies.newest?.find((b) => b.EntityID === this.you)
  }

  /** Follows the stream: returns the join frame on each connect, and starts over on leaving it. */
  setConnected(connected: boolean, move: Vec2): Input | undefined {
    if (connected === this.#connected) return undefined
    this.#connected = connected
    if (connected) return this.input.join(move)
    this.#timelines.reset()
    return undefined
  }

  onBodySync(s: BodySync): void {
    this.#timelines.add(this.bodies, s.Tick, s.Bodies)
    this.received++
    this.you = s.You.Body
    this.tickSeconds = s.TickSeconds
    this.input.ack(s.You)
    if (this.tickSeconds > 0 && this.me) this.#playing = true
  }

  /** The Inputs to send this animation frame. Aim is from your Body's newest server position. */
  inputsToSend(dt: number, held: Held): Input[] {
    if (!this.#connected) return []
    if (!this.#playing) {
      const input = this.input.moveChanged(held.move)
      return input ? [input] : []
    }
    const { move, ground, buttons } = held
    const me = this.me
    const aim = ground && me ? { x: ground.x - me.X, y: ground.y - me.Y } : { x: 0, y: 0 }
    return this.input.step(dt, this.tickSeconds, {
      MoveX: move.x,
      MoveY: move.y,
      AimX: aim.x,
      AimY: aim.y,
      Buttons: buttons,
    })
  }
}
