import type { Vec2 } from '../../engine/math/vec2'
import { blend } from '../../engine/render/entity_views'
import type { EchoSync } from '../echo/echo_sync_event'
import type { BodyState, BodySync } from './body_sync_event'
import type { HitClaim, InputFrame } from './input_command'
import type { EndedShot, PredictedKit, PredictedShot, Predictor } from './predictor'

const FadeSeconds = 0.1 // a correction shrinks to 1/e in this long
const SnapDistance = 3 // world units; a bigger correction jumps instead of fading

/** One of your shots, where to draw it. */
export interface DrawnShot {
  Type: string
  X: number
  Y: number
  Radius: number
}

/** A shot's key: the frame that cast it, the ability, and its place among that cast's launches. */
const keyOf = (s: { Seq: number; Ability: string; Launch: number }) => `${s.Seq} ${s.Ability} ${s.Launch}`

/**
 * Your Body ahead of the server, with what it casts. Each frame you send runs one tick at once,
 * through the server's own code: it moves your Body, casts, and launches and flies your shots. A shot
 * that stops on a Body or Echo where your screen drew it ends there, and the frame claims the hit. Each
 * body_sync starts it over from the server's state after your last applied frame, then replays the
 * frames the server hasn't applied. With no surprise that lands on the same spot; when it doesn't, the
 * drawn Body fades to the new one instead of jumping. Your shots are rebuilt the same way, so a shot
 * the server ended is gone after the next body_sync, and one you ended never comes back. A shot that
 * ends in a tick still flies that tick, up to where it ended.
 */
export class Prediction {
  readonly #predictor: Predictor
  #at: Vec2 | undefined // after the newest frame
  #from: Vec2 = { x: 0, y: 0 } // drawn at the start of the newest tick
  #to: Vec2 = { x: 0, y: 0 } // drawn at its end: #at plus what's left of a correction
  #shots: PredictedShot[] = []
  #kit: PredictedKit | undefined
  readonly #ended = new Set<string>() // keys of the shots your frames claimed
  #ending: EndedShot[] = [] // the shots that ended in the newest tick; a body_sync keeps them
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

  /** Keeps the Bodies a body_sync shows, so your shots meet them where your screen draws them. */
  seeBodies(sync: BodySync): void {
    this.#predictor.seeBodies(sync)
  }

  /** Keeps the Echoes an echo_sync shows, the same way. */
  seeEchoes(sync: EchoSync): void {
    this.#predictor.seeEchoes(sync)
  }

  /**
   * Starts over from sync, then replays the frames the server hasn't applied. A shot your frames claimed
   * stays gone while a frame that claims it is unacked or the server still lists it, so a claim the
   * server rejects can't bring it back.
   */
  reconcile(sync: BodySync, you: BodyState, unacked: readonly InputFrame[]): void {
    const claimed = new Set(unacked.flatMap((f) => f.Claims.map(keyOf)))
    const listed = new Set(sync.You.Shots.map(keyOf))
    for (const key of this.#ended) if (!claimed.has(key) && !listed.has(key)) this.#ended.delete(key)
    const shots = sync.You.Shots.filter((s) => !this.#ended.has(keyOf(s)))
    this.#predictor.reset({ ...sync, You: { ...sync.You, Shots: shots } }, you)
    let at: Vec2 = { x: you.X, y: you.Y }
    for (const f of unacked) at = this.#predictor.replay(f)
    const was = this.#at
    this.#at = at
    if (was && (at.x !== was.x || at.y !== was.y)) this.corrections++
    if (!was || Math.hypot(this.#to.x - at.x, this.#to.y - at.y) > SnapDistance) {
      this.#from = this.#to = at
    }
    this.#read()
  }

  /** Where a frame moving (moveX, moveY) would leave your Body, without stepping: aim from there. */
  peek(moveX: number, moveY: number): Vec2 {
    if (!this.#at) throw new Error('Prediction.peek before the first body_sync')
    return this.#predictor.peek(moveX, moveY)
  }

  /** Runs one tick of a frame you're sending, and returns its claims: the shots it ended on a Body or Echo. */
  step(frame: InputFrame, tickSeconds: number): HitClaim[] {
    const was = this.#at
    if (!was) throw new Error('Prediction.step before the first body_sync')
    const { at, claims, ended } = this.#predictor.step(frame)
    for (const c of claims) this.#ended.add(keyOf(c))
    this.#ending = ended
    const keep = Math.exp(-tickSeconds / FadeSeconds)
    this.#from = this.#to
    this.#to = { x: at.x + (this.#to.x - was.x) * keep, y: at.y + (this.#to.y - was.y) * keep }
    this.#at = at
    this.#read()
    return claims
  }

  /** Where to draw your Body, alpha (0 to 1) of the way through the newest tick. */
  drawn(alpha: number): Vec2 {
    const from = this.#from
    const to = this.#to
    return { x: from.x + (to.x - from.x) * alpha, y: from.y + (to.y - from.y) * alpha }
  }

  /**
   * Where to draw your shots, alpha (0 to 1) of the way through the newest tick. One that ended in it
   * shows until it gets there.
   */
  shots(alpha: number): DrawnShot[] {
    const drawn: DrawnShot[] = []
    const draw = (s: PredictedShot, until?: number) => {
      const at = blend({ X: s.PrevX, Y: s.PrevY }, s, alpha, until)
      if (at) drawn.push({ Type: s.Type, X: at.x, Y: at.y, Radius: s.Radius })
    }
    for (const s of this.#shots) draw(s)
    for (const s of this.#ending) draw(s, s.Until)
    return drawn
  }

  /** Forgets your Body and what the syncs showed, after a reconnect or a server restart. */
  clear(): void {
    this.#at = undefined
    this.#shots = []
    this.#kit = undefined
    this.#ended.clear()
    this.#ending = []
    this.#predictor.forget()
  }

  #read(): void {
    this.#shots = this.#predictor.shots()
    this.#kit = this.#predictor.kit()
  }
}
