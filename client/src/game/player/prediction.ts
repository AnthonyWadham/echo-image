import type { Vec2 } from '../../engine/math/vec2'
import type { BodyState, BodySync } from './body_sync_event'
import type { InputFrame } from './input_command'
import type { FrameInput } from './input_stream'
import type { PredictedKit, PredictedShot, Predictor } from './predictor'

const FadeSeconds = 0.1 // a correction shrinks to 1/e in this long
const SnapDistance = 3 // world units; a bigger correction jumps instead of fading

/** One of your shots, where to draw it. */
export interface DrawnShot {
  Type: string
  X: number
  Y: number
  Radius: number
}

/**
 * Your Body ahead of the server, with what it casts. Each frame you send runs one tick at once,
 * through the server's own code: it moves your Body, casts, and launches and flies your shots. Each
 * body_sync starts it over from the server's state after your last applied frame, then replays the
 * frames the server hasn't applied. With no surprise that lands on the same spot; when it doesn't, the
 * drawn Body fades to the new one instead of jumping. Your shots are rebuilt the same way, so a shot
 * the server ended (it hit something) is gone after the next body_sync.
 */
export class Prediction {
  readonly #predictor: Predictor
  #at: Vec2 | undefined // after the newest frame
  #from: Vec2 = { x: 0, y: 0 } // drawn at the start of the newest tick
  #to: Vec2 = { x: 0, y: 0 } // drawn at its end: #at plus what's left of a correction
  #shots: PredictedShot[] = []
  #kit: PredictedKit | undefined
  /** body_syncs whose replay didn't land where the frames had put your Body. */
  corrections = 0

  constructor(predictor: Predictor) {
    this.#predictor = predictor
  }

  /** Where your Body is after the newest frame, or undefined before a body_sync. */
  get position(): Vec2 | undefined {
    return this.#at
  }

  /** Your kit after the newest frame, or undefined before a body_sync. */
  get kit(): PredictedKit | undefined {
    return this.#kit
  }

  /** Starts over from sync, then replays the frames the server hasn't applied. */
  reconcile(sync: BodySync, you: BodyState, unacked: readonly InputFrame[]): void {
    // DEBUG(shots): temporary, remove after the off-screen shot hunt.
    const before = this.#shots.length
    console.debug('[shots] body_sync', sync.Tick, 'server', sync.You.Shots, 'unacked', unacked.length)
    this.#predictor.reset(sync, you)
    let at: Vec2 = { x: you.X, y: you.Y }
    for (const f of unacked) at = this.#predictor.step(f)
    const was = this.#at
    this.#at = at
    if (was && (at.x !== was.x || at.y !== was.y)) this.corrections++
    if (!was || Math.hypot(this.#to.x - at.x, this.#to.y - at.y) > SnapDistance) {
      this.#from = this.#to = at
    }
    this.#read()
    if (before || this.#shots.length) {
      console.debug('[shots] reconciled', before, '->', this.#shots.length, this.#shots.map((s) => [s.X, s.Y]))
    }
  }

  /** Where a frame moving (moveX, moveY) would leave your Body, without stepping: aim from there. */
  peek(moveX: number, moveY: number): Vec2 {
    if (!this.#at) throw new Error('Prediction.peek before the first body_sync')
    return this.#predictor.peek(moveX, moveY)
  }

  /** Runs one tick of a frame you're sending, and returns where it leaves your Body. */
  step(frame: FrameInput, tickSeconds: number): Vec2 {
    const was = this.#at
    if (!was) throw new Error('Prediction.step before the first body_sync')
    const at = this.#predictor.step(frame)
    const keep = Math.exp(-tickSeconds / FadeSeconds)
    this.#from = this.#to
    this.#to = { x: at.x + (this.#to.x - was.x) * keep, y: at.y + (this.#to.y - was.y) * keep }
    this.#at = at
    this.#read()
    return at
  }

  /** Where to draw your Body, alpha (0 to 1) of the way through the newest tick. */
  drawn(alpha: number): Vec2 {
    const from = this.#from
    const to = this.#to
    return { x: from.x + (to.x - from.x) * alpha, y: from.y + (to.y - from.y) * alpha }
  }

  /** Where to draw your shots, alpha (0 to 1) of the way through the newest tick. */
  shots(alpha: number): DrawnShot[] {
    return this.#shots.map((s) => ({
      Type: s.Type,
      X: s.PrevX + (s.X - s.PrevX) * alpha,
      Y: s.PrevY + (s.Y - s.PrevY) * alpha,
      Radius: s.Radius,
    }))
  }

  /** Forgets your Body, after a reconnect or a server restart. */
  clear(): void {
    this.#at = undefined
    this.#shots = []
    this.#kit = undefined
  }

  #read(): void {
    this.#shots = this.#predictor.shots()
    this.#kit = this.#predictor.kit()
  }
}
