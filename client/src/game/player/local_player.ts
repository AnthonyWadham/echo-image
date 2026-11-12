import type { Vec2 } from '../../engine/math/vec2'
import type { Spot } from '../../engine/render/entity_views'
import type { Timeline, Timelines } from '../../engine/sync/timeline'
import type { TargetState, TargetSync } from '../track/target_sync_event'
import type { BodyState, BodySync, InputHealth } from './body_sync_event'
import type { Input } from './input_command'
import { InputStream } from './input_stream'
import { Prediction } from './prediction'
import type { DrawnShot } from './prediction'
import type { EndedShot, PredictedKit, Predictor } from './predictor'

/** What the player holds this frame. ground is the point on the ground under the mouse. */
export interface Held {
  move: Vec2
  ground: Vec2 | undefined
  /** InputFrame.Buttons for the next frame (bit N casts kit slot N): held, or pressed since a frame took them. */
  takeButtons: () => number
  /** The render tick on screen: the last one drawn, if any. */
  view: number | undefined
}

/** You: every Body's recent states, which one is yours, the Input you send, and your Body predicted. */
export class LocalPlayer {
  /** What each body_sync shows of every Body beyond its row: who plays it and how it looks. */
  readonly bodies: Timeline<BodyState[]>
  /**
   * The rows each target_sync shows, every Body and Echo's: each kind's views draw theirs. The render
   * clock waits for them, so your predictor has every view's rows.
   */
  readonly targets: Timeline<TargetState[]>
  readonly input: InputStream
  readonly prediction: Prediction
  /** body_sync events received. */
  received = 0
  /** Your Body's EntityID, from the newest body_sync; undefined before one, since 0 is a Body too. */
  you: number | undefined
  /** Your team, from the newest body_sync. */
  team: number | undefined
  /** What went wrong with your input stream, as the newest body_sync counts it. */
  inputHealth: InputHealth | undefined
  tickSeconds = 0
  /** The sim your predictor runs. */
  readonly sim: string
  /** The sim the shard runs, while it isn't yours: you don't play until it is. */
  otherSim: string | undefined
  readonly #timelines: Timelines
  #connected = false
  #playing = false
  #sameSim = false // the newest body_sync since connecting says the shard runs your sim
  #own: { body?: bigint; echo?: bigint } = {} // the serials of what you predict, from the newest body_sync

  constructor(timelines: Timelines, session: number, predictor: Predictor) {
    this.#timelines = timelines
    this.bodies = timelines.create<BodyState[]>()
    this.targets = timelines.create<TargetState[]>()
    this.input = new InputStream(session)
    this.prediction = new Prediction(predictor)
    this.sim = predictor.version
    timelines.onReset(() => {
      this.#playing = false
      this.#sameSim = false
      this.#own = {}
      this.input.reset()
      this.prediction.clear()
    })
  }

  /** The server has given us a Body: input goes out every tick. */
  get playing(): boolean {
    return this.#playing
  }

  /** What the newest body_sync shows of your Body beyond its row. */
  get me(): BodyState | undefined {
    return this.bodies.newest?.find((b) => b.EntityID === this.you)
  }

  /** Where to draw your Body while playing: predicted, partway through the newest tick. */
  get drawn(): Vec2 | undefined {
    return this.#playing ? this.prediction.drawn(this.#alpha) : undefined
  }

  /** Where to draw your Echo while playing: predicted like your Body, so the gap between them is the real one. */
  get drawnEcho(): Vec2 | undefined {
    return this.#playing ? this.prediction.echo(this.#alpha) : undefined
  }

  /** Whether serial names your Body. */
  isYou(serial: bigint): boolean {
    return serial === this.#own.body
  }

  /** Where to draw serial's thing while playing, if it is your Body or Echo: by serial, never kind or owner. */
  drawnOwn(serial: bigint): Vec2 | undefined {
    if (serial === this.#own.body) return this.drawn
    if (serial === this.#own.echo) return this.drawnEcho
    return undefined
  }

  /**
   * Where to draw your shots: predicted like your Body, so each leaves the moment you cast it. end says
   * where one that ended on a Body or Echo is drawn ending.
   */
  drawnShots(end?: (s: EndedShot) => Spot | undefined): DrawnShot[] {
    return this.#playing ? this.prediction.shots(this.#alpha, end) : []
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
    this.received++
    this.#timelines.tickSeconds = s.TickSeconds
    // Your predictor runs its own sim: against another, it would mispredict every frame.
    this.#sameSim = s.SimVersion === this.sim
    if (!this.#sameSim) {
      this.otherSim = s.SimVersion
      this.#playing = false
      return
    }
    this.otherSim = undefined
    this.#timelines.add(this.bodies, s.Tick, s.Bodies)
    this.you = s.You.Body
    this.team = s.You.Team
    this.inputHealth = s.You.InputHealth
    this.tickSeconds = s.TickSeconds
    this.#own = { body: this.me?.Serial, echo: s.You.EchoSerial > 0n ? s.You.EchoSerial : undefined }
    this.input.ack(s.You, s.TickSeconds)
    if (!(this.tickSeconds > 0 && this.me)) return
    this.#playing = true
    this.prediction.reconcile(s, this.input.unacked, s.You.InputSession === this.input.session)
  }

  /** Keeps the rows each target_sync shows, to draw, and for your shots to meet once the shard runs your sim. */
  onTargetSync(s: TargetSync): void {
    this.#timelines.add(this.targets, s.Tick, s.Targets)
    if (this.#sameSim) this.prediction.seeTargets(s)
  }

  /** The Inputs to send this animation frame. Each frame runs your predicted Body one tick. */
  inputsToSend(dt: number, held: Held): Input[] {
    if (!this.#playing) held.takeButtons() // so a press made while you can't play casts nothing later
    if (!this.#connected || this.otherSim !== undefined) return []
    if (!this.#playing) {
      const input = this.input.moveChanged(held.move)
      return input ? [input] : []
    }
    const { move, ground, view } = held
    return this.input.step(dt, this.tickSeconds, (seq) => {
      // The server casts from where this frame moves your Body, so aim from there.
      const at = this.prediction.peek(move.x, move.y)
      const aim = ground ? { x: ground.x - at.x, y: ground.y - at.y } : { x: 0, y: 0 }
      const buttons = held.takeButtons()
      const frame = { MoveX: move.x, MoveY: move.y, AimX: aim.x, AimY: aim.y, Buttons: buttons, ViewTick: view ?? 0 }
      this.prediction.step({ Seq: seq, ...frame }, this.tickSeconds)
      return frame
    })
  }
}
