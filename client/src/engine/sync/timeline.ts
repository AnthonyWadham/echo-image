// Server time: ticks off the wire, each sync event's recent states, and the clock they're drawn at.

const MaxStates = 64 // 3.2 s at 20 Hz
const MaxDrift = 0.25 // seconds; past this, the render clock jumps to its target
const MaxSafeTick = BigInt(Number.MAX_SAFE_INTEGER)

/** Seconds behind the newest state to draw: about two ticks, so a newer state is usually there. */
export const InterpolationDelay = 0.1

/** A Go uint64 tick as a number. Throws past Number.MAX_SAFE_INTEGER, where it loses precision. */
export function toTick(tick: bigint): number {
  if (tick > MaxSafeTick) throw new RangeError(`tick ${tick} exceeds Number.MAX_SAFE_INTEGER`)
  return Number(tick)
}

/** The render clock after dt: eased 5% toward target, or at target when unset or too far off. */
export function stepRenderTime(renderTime: number, target: number, dt: number): number {
  if (Number.isNaN(renderTime)) return target
  const next = renderTime + dt
  return Math.abs(next - target) > MaxDrift ? target : next + (target - next) * 0.05
}

/** The two states around a tick, and how far the tick is between them (0 = from, 1 = to). */
export interface Sample<T> {
  from: T
  to: T
  t: number
}

const clamp01 = (x: number) => Math.min(Math.max(x, 0), 1)

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
    if (this.#states.length > MaxStates) this.#states.shift()
    return true
  }

  clear(): void {
    this.#states.length = 0
  }

  /** The two states around tick. States older than both are dropped. */
  sample(tick: number): Sample<T> | undefined {
    const states = this.#states
    while (states.length > 2 && states[1]!.tick <= tick) states.shift()
    const from = states[0]
    if (!from) return undefined
    const to = states[1] ?? from
    const span = to.tick - from.tick
    return { from: from.state, to: to.state, t: span > 0 ? clamp01((tick - from.tick) / span) : 1 }
  }
}

/** Every sync event's timeline. They start over together and are drawn on one render clock. */
export class Timelines {
  readonly #all: Timeline<unknown>[] = []
  readonly #onReset: (() => void)[] = []
  #renderTime = NaN

  /** A new timeline that starts over with the rest. */
  create<T>(): Timeline<T> {
    const timeline = new Timeline<T>()
    this.#all.push(timeline)
    return timeline
  }

  /** Adds a state at a uint64 tick. A tick going backwards (a server restart) resets them all. */
  add<T>(timeline: Timeline<T>, tick: bigint, state: T): void {
    const at = toTick(tick)
    if (timeline.add(at, state)) return
    this.reset()
    timeline.add(at, state)
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

  /** The tick to draw, dt after the last: InterpolationDelay behind clock's newest state. */
  renderTick(dt: number, clock: Timeline<unknown>, tickSeconds: number): number | undefined {
    if (clock.size === 0 || !(tickSeconds > 0)) return undefined
    const target = clock.newestTick * tickSeconds - InterpolationDelay
    this.#renderTime = stepRenderTime(this.#renderTime, target, dt)
    return this.#renderTime / tickSeconds
  }
}
