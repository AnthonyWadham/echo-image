import type { Vec2 } from '../../engine/math/vec2'
import { blend } from '../../engine/render/entity_views'
import type { Spot } from '../../engine/render/entity_views'
import type { TargetSync } from '../projectile/target_sync_event'
import type { BodyState, BodySync } from './body_sync_event'
import type { InputFrame } from './input_command'
import type { EndedShot, PredictedEcho, PredictedKit, PredictedShot, Predictor } from './predictor'

const FadeSeconds = 0.1 // a correction shrinks to 1/e in this long
const SnapDistance = 3 // world units; a bigger correction jumps instead of fading

const keyOf = (s: PredictedShot) => `${s.Seq} ${s.Ability} ${s.Launch}`
const startOf = (s: { PrevX: number; PrevY: number }): Spot => ({ X: s.PrevX, Y: s.PrevY })
const spotOf = (v: Vec2): Spot => ({ X: v.x, Y: v.y })
const plus = (a: Spot, b: Spot): Spot => ({ X: a.X + b.X, Y: a.Y + b.Y })
const near = (a: Spot, b: Spot) => Math.hypot(a.X - b.X, a.Y - b.Y) <= SnapDistance
const none: Spot = { X: 0, Y: 0 }

/** Where a predicted thing is drawn as the newest tick starts, and as its path ends: an ended shot's end. */
interface Path {
  from: Spot
  to: Spot
}

/** A predicted thing after the newest tick, and its path through it. */
interface Drawn<T> extends Path {
  now: T
}

/** Where a path is drawn, alpha (0 to 1) of the way through the newest tick. */
const along = (p: Path, alpha: number): Vec2 => ({
  x: p.from.X + (p.to.X - p.from.X) * alpha,
  y: p.from.Y + (p.to.Y - p.from.Y) * alpha,
})

/** One of your shots, where to draw it. */
export interface DrawnShot {
  Type: string
  X: number
  Y: number
  Radius: number
}

/**
 * Your Body ahead of the server, with what it casts. Each frame you send runs one tick at once,
 * through the server's own code: it moves your Body, casts, and launches and flies your shots. A shot
 * meets Bodies and Echoes where your screen drew them, as the server judges it. Each body_sync starts it
 * over from the server's state after your last applied frame, then replays the frames the server
 * hasn't applied. With no surprise that lands on the same spot; when it doesn't, your Body, your Echo
 * and each shot fade to their new spots instead of jumping, and a shot the server never launched is
 * gone. A shot that ends in a tick still flies that tick, up to where it ended.
 */
export class Prediction {
  readonly #predictor: Predictor
  #body: Drawn<Vec2> | undefined // your Body after the newest frame
  #shots: Drawn<PredictedShot>[] = []
  #ending: Drawn<EndedShot>[] = [] // the shots that ended in the newest tick; a body_sync replays or keeps them
  #echo: Drawn<PredictedEcho> | undefined
  #kit: PredictedKit | undefined
  /** body_syncs whose replay didn't land where the frames had put your Body. */
  corrections = 0

  constructor(predictor: Predictor) {
    this.#predictor = predictor
  }

  /** Where your Body is after the newest frame, or undefined before a body_sync. */
  get position(): Vec2 | undefined {
    return this.#body?.now
  }

  /** Your kit after the newest frame, or undefined before a body_sync. */
  get kit(): PredictedKit | undefined {
    return this.#kit
  }

  /** Keeps the targets a target_sync shows, so your shots meet them where your screen draws them. */
  seeTargets(sync: TargetSync): void {
    this.#predictor.seeTargets(sync)
  }

  /** Starts over from sync, then replays the frames the server hasn't applied. */
  reconcile(sync: BodySync, you: BodyState, unacked: readonly InputFrame[]): void {
    this.#predictor.reset(sync, you)
    let at: Vec2 = { x: you.X, y: you.Y }
    let ended: EndedShot[] | undefined
    for (const f of unacked) {
      const stepped = this.#predictor.step(f)
      at = stepped.at
      ended = stepped.ended
    }
    const body = this.#body
    if (body && (at.x !== body.now.x || at.y !== body.now.y)) this.corrections++
    // The replay ends in the same tick, so each thing stays on the path it's drawn on and fades from there,
    // unless that path ends more than SnapDistance from its new spot: then it starts where it now is, as
    // does a thing not drawn before.
    const stay = <T>(now: T, start: Spot, end: Spot, was: Path | undefined): Drawn<T> =>
      was && near(was.to, end) ? { now, from: was.from, to: was.to } : { now, from: start, to: end }
    const here = spotOf(at)
    this.#body = stay(at, here, here, body)
    const shots = this.#predictor.shots()
    if (ended) {
      const drawn = new Map([...this.#shots, ...this.#ending].map((d) => [keyOf(d.now), d]))
      this.#ending = ended.map((e) => {
        const was = drawn.get(keyOf(e))
        return { now: e, from: was && near(was.from, startOf(e)) ? was.from : startOf(e), to: e }
      })
    } else {
      // Nothing replayed, so the newest tick is the sync's: keep the ends your frames drew in it, but
      // not that of a shot the server flew on.
      const flies = new Set(shots.map(keyOf))
      this.#ending = this.#ending.filter((d) => !flies.has(keyOf(d.now)))
    }
    // Only a shot drawn flying keeps its path: one drawn ending stops partway through the tick.
    const flying = new Map(this.#shots.map((d) => [keyOf(d.now), d]))
    this.#shots = shots.map((s) => stay(s, startOf(s), s, flying.get(keyOf(s))))
    const echo = this.#predictor.echo()
    this.#echo = echo && stay(echo, startOf(echo), echo, this.#echo)
    this.#kit = this.#predictor.kit()
  }

  /** Where a frame moving (moveX, moveY) would leave your Body, without stepping: aim from there. */
  peek(moveX: number, moveY: number): Vec2 {
    if (!this.#body) throw new Error('Prediction.peek before the first body_sync')
    return this.#predictor.peek(moveX, moveY)
  }

  /** Runs one tick of a frame you're sending. */
  step(frame: InputFrame, tickSeconds: number): void {
    const body = this.#body
    if (!body) throw new Error('Prediction.step before the first body_sync')
    const { at, ended } = this.#predictor.step(frame)
    // Each thing goes on from where it was drawn, and what that is off its true spot shrinks by keep. A
    // new shot starts as far off as your Body (lead), so it leaves where your Body is drawn.
    const keep = Math.exp(-tickSeconds / FadeSeconds)
    const onward = <T>(now: T, start: Spot, end: Spot, was: Path | undefined, lead = none): Drawn<T> => {
      if (!was) return { now, from: plus(start, lead), to: plus(end, lead) }
      const to = { X: end.X + (was.to.X - start.X) * keep, Y: end.Y + (was.to.Y - start.Y) * keep }
      return { now, from: was.to, to }
    }
    this.#body = onward(at, spotOf(body.now), spotOf(at), body)
    const lead = { X: this.#body.to.X - at.x, Y: this.#body.to.Y - at.y }
    const flying = new Map(this.#shots.map((d) => [keyOf(d.now), d]))
    this.#shots = this.#predictor.shots().map((s) => onward(s, startOf(s), s, flying.get(keyOf(s)), lead))
    this.#ending = ended.map((e) => ({ ...onward(e, startOf(e), e, flying.get(keyOf(e)), lead), to: e }))
    const echo = this.#predictor.echo()
    this.#echo = echo && onward(echo, startOf(echo), echo, this.#echo)
    this.#kit = this.#predictor.kit()
  }

  /** Where to draw your Body, alpha (0 to 1) of the way through the newest tick. */
  drawn(alpha: number): Vec2 {
    if (!this.#body) throw new Error('Prediction.drawn before the first body_sync')
    return along(this.#body, alpha)
  }

  /**
   * Where to draw your shots, alpha (0 to 1) of the way through the newest tick. One that ended in it
   * shows until it gets there: where it ended, or where end puts it.
   */
  shots(alpha: number, end?: (s: EndedShot) => Spot | undefined): DrawnShot[] {
    const drawn: DrawnShot[] = []
    const draw = ({ now: s, from }: Drawn<PredictedShot>, to: Spot, until?: number) => {
      const at = blend(from, to, alpha, until)
      if (at) drawn.push({ Type: s.Type, X: at.x, Y: at.y, Radius: s.Radius })
    }
    for (const d of this.#shots) draw(d, d.to)
    for (const d of this.#ending) draw(d, end?.(d.now) ?? d.to, d.now.Until)
    return drawn
  }

  /** Where to draw your Echo, alpha (0 to 1) of the way through the newest tick, or undefined with none. */
  echo(alpha: number): Vec2 | undefined {
    return this.#echo && along(this.#echo, alpha)
  }

  /** Forgets your Body and what the syncs showed, after a reconnect or a server restart. */
  clear(): void {
    this.#body = undefined
    this.#shots = []
    this.#ending = []
    this.#echo = undefined
    this.#kit = undefined
    this.#predictor.forget()
  }
}
