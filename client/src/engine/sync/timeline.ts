// Server time: ticks off the wire, each sync event's recent states, and the clock they're drawn at.

const MaxDrift = 0.25 // seconds behind its target past which the render clock jumps ahead to it
const EaseSeconds = 0.5 // the render clock closes 63% of its gap to its target in this time
const MaxSafeTick = BigInt(Number.MAX_SAFE_INTEGER)

/** Seconds behind the newest tick to draw: about two ticks, so the clock seldom reaches it and waits. */
export const InterpolationDelay = 0.1

/** The farthest back of the newest tick a view is drawn, in seconds: older states are dropped. */
const KeepSeconds = MaxDrift + InterpolationDelay

/** A Go uint64 tick as a number. Throws past Number.MAX_SAFE_INTEGER, where it loses precision. */
export function toTick(tick: bigint): number {
  if (tick > MaxSafeTick) throw new RangeError(`tick ${tick} exceeds Number.MAX_SAFE_INTEGER`)
  return Number(tick)
}

/**
 * The render clock dt later. It runs at real time, eased toward target by time, not by frame, and jumps
 * ahead to target when more than MaxDrift behind. It never steps back, and never passes newest: the
 * newest tick's time every timeline has, so each view it draws has both its states.
 */
export function stepRenderTime(renderTime: number, target: number, newest: number, dt: number): number {
  if (Number.isNaN(renderTime) || target - renderTime > MaxDrift) return Math.min(target, newest)
  const next = renderTime + dt
  const eased = next + (target - next) * (1 - Math.exp(-dt / EaseSeconds))
  return Math.min(Math.max(eased, renderTime), newest)
}

/** The two states a view is drawn from, and how far it is through the newer one's tick (0 < t <= 1). */
export interface Sample<T> {
  from: T
  to: T
  t: number
}

/** One sync event's recent states, oldest first, so it can be drawn between two of them. */
export class Timeline<T> {
  readonly #states: { tick: number; state: T }[] = []

  get size(): number {
    return this.#states.length
  }

  get newest(): T | undefined {
    return this.#states.at(-1)?.state
  }

  /** The newest state's tick, or 0 when empty. */
  get newestTick(): number {
    return this.#states.at(-1)?.tick ?? 0
  }

  /** Adds a state; false if tick went backwards (the server restarted). A repeat is ignored. */
  add(tick: number, state: T): boolean {
    const newest = this.#states.at(-1)
    if (newest && tick < newest.tick) return false
    if (!newest || tick > newest.tick) this.#states.push({ tick, state })
    return true
  }

  /** Drops the states before tick. */
  dropBefore(tick: number): void {
    let n = 0
    while (n < this.#states.length && this.#states[n]!.tick < tick) n++
    this.#states.splice(0, n)
  }

  clear(): void {
    this.#states.length = 0
  }

  /**
   * The states view v is drawn from: those at ticks ⌈v⌉ - 1 and ⌈v⌉, so a whole tick is its own end.
   * Undefined without both. The server judges your shots by the same rule (flight.RowTicks, AtView).
   */
  sample(v: number): Sample<T> | undefined {
    const b = Math.ceil(v)
    const from = this.#at(b - 1)
    const to = this.#at(b)
    if (!from || !to) return undefined
    return { from: from.state, to: to.state, t: v - from.tick }
  }

  // The state kept at tick, if any. Views are drawn near the newest, so it looks from there.
  #at(tick: number): { tick: number; state: T } | undefined {
    for (let i = this.#states.length - 1; i >= 0; i--) {
      const s = this.#states[i]!
      if (s.tick <= tick) return s.tick === tick ? s : undefined
    }
    return undefined
  }
}

/** Every sync event's timeline. They start over together and are drawn on one render clock. */
export class Timelines {
  /** Seconds a tick lasts (body_sync's TickSeconds); until it's known, no state is dropped. */
  tickSeconds = 0
  readonly #all: Timeline<unknown>[] = []
  readonly #onReset: (() => void)[] = []
  #renderTime = NaN

  /** A new timeline that starts over with the rest. */
  create<T>(): Timeline<T> {
    const timeline = new Timeline<T>()
    this.#all.push(timeline)
    return timeline
  }

  /**
   * Adds a state at a uint64 tick. A tick going backwards (a server restart) resets them all. Then each
   * drops what no view can draw again: states over KeepSeconds older than the newest tick every timeline
   * has, but the one a view there pairs with. So each holds the same seconds at any rate, hidden tab
   * included.
   */
  add<T>(timeline: Timeline<T>, tick: bigint, state: T): void {
    const at = toTick(tick)
    if (!timeline.add(at, state)) {
      this.reset()
      timeline.add(at, state)
    }
    const newest = this.newestTick
    if (newest === undefined || !(this.tickSeconds > 0)) return
    const oldest = newest - Math.ceil(KeepSeconds / this.tickSeconds) - 1
    for (const t of this.#all) t.dropBefore(oldest)
  }

  /** Calls listener after each reset, for state learned along with the timelines. */
  onReset(listener: () => void): void {
    this.#onReset.push(listener)
  }

  /** Forgets everything learned from the server: after a reconnect or a server restart. */
  reset(): void {
    for (const timeline of this.#all) timeline.clear()
    this.#renderTime = NaN
    for (const listener of this.#onReset) listener()
  }

  /** The newest tick every timeline has, or undefined until each has a state. */
  get newestTick(): number | undefined {
    if (this.#all.length === 0 || this.#all.some((t) => t.size === 0)) return undefined
    return Math.min(...this.#all.map((t) => t.newestTick))
  }

  /** The tick to draw, dt after the last: InterpolationDelay behind newestTick, and never past it. */
  renderTick(dt: number, tickSeconds: number): number | undefined {
    const newest = this.newestTick
    if (newest === undefined || !(tickSeconds > 0)) return undefined
    const at = newest * tickSeconds
    this.#renderTime = stepRenderTime(this.#renderTime, at - InterpolationDelay, at, dt)
    return this.#renderTime / tickSeconds
  }
}
