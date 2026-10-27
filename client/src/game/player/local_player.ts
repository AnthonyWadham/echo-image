import type { Vec2 } from '../../engine/math/vec2'
import type { Timeline, Timelines } from '../../engine/sync/timeline'
import type { BodyState, BodySync } from './body_sync_event'
import type { Input } from './input_command'
import { InputStream } from './input_stream'
import { Prediction } from './prediction'
import type { DrawnShot } from './prediction'
import type { PredictedKit, Predictor } from './predictor'

/** What the player holds this frame. ground is the point on the ground under the mouse. */
export interface Held {
  move: Vec2
  ground: Vec2 | undefined
  /** InputFrame.Buttons: bit N casts kit slot N. */
  buttons: number
}

/** You: every Body's recent states, which one is yours, the Input you send, and your Body predicted. */
export class LocalPlayer {
  readonly bodies: Timeline<BodyState[]>
  readonly input: InputStream
  readonly prediction: Prediction
  /** body_sync events received. */
  received = 0
  /** Your Body's EntityID, from the newest body_sync. */
  you = 0
  tickSeconds = 0
  readonly #timelines: Timelines
  #connected = false
  #playing = false

  constructor(timelines: Timelines, session: number, predictor: Predictor) {
    this.#timelines = timelines
    this.bodies = timelines.create<BodyState[]>()
    this.input = new InputStream(session)
    this.prediction = new Prediction(predictor)
    timelines.onReset(() => {
      this.#playing = false
      this.input.reset()
      this.prediction.clear()
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

  /** Where to draw your Body while playing: predicted, partway through the newest tick. */
  get drawn(): Vec2 | undefined {
    return this.#playing ? this.prediction.drawn(this.#alpha) : undefined
  }

  /** Where to draw your shots: predicted like your Body, so each leaves the moment you cast it. */
  get drawnShots(): DrawnShot[] {
    return this.#playing ? this.prediction.shots(this.#alpha) : []
  }

  /** Your kit's cooldowns while playing, predicted, so a cast starts its cooldown at once. */
  get kit(): PredictedKit | undefined {
    return this.#playing ? this.prediction.kit : undefined
  }

  // How far through the newest tick to draw.
  get #alpha(): number {
    return this.input.stepTime / this.tickSeconds
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
    const me = this.me
    if (!(this.tickSeconds > 0 && me)) return
    this.#playing = true
    this.prediction.reconcile(s, me, this.input.unacked)
  }

  /** The Inputs to send this animation frame. Each frame runs your predicted Body one tick. */
  inputsToSend(dt: number, held: Held): Input[] {
    if (!this.#connected) return []
    if (!this.#playing) {
      const input = this.input.moveChanged(held.move)
      return input ? [input] : []
    }
    const { move, ground, buttons } = held
    return this.input.step(dt, this.tickSeconds, () => {
      // The server casts from where this frame moves your Body, so aim from there.
      const at = this.prediction.peek(move.x, move.y)
      const aim = ground ? { x: ground.x - at.x, y: ground.y - at.y } : { x: 0, y: 0 }
      const frame = { MoveX: move.x, MoveY: move.y, AimX: aim.x, AimY: aim.y, Buttons: buttons }
      this.prediction.step(frame, this.tickSeconds)
      return frame
    })
  }
}
