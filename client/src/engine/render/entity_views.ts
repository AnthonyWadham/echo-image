import type { Object3D } from 'three'
import type { Vec2 } from '../math/vec2'
import type { Timeline } from '../sync/timeline'

/**
 * What a view needs of an entity's state. Serial names it across ticks: an EntityID can name a newer
 * entity a tick later, a serial never. Until is the part of the tick it lived, 1 if unset: one that ended
 * in it is at X, Y. PrevX, PrevY, if set, is where it was as the tick began: it is drawn from there, not
 * from its older state, so it leaves where it was made.
 */
export interface Placed {
  Serial: bigint
  PrevX?: number
  PrevY?: number
  X: number
  Y: number
  Until?: number
}

/** A state's position. */
export type Spot = { X: number; Y: number }

/**
 * Where to draw an entity t (0 to 1) through a tick, blended from its older state p to s. One that ended
 * partway through (until < 1) moves at its own pace to s, and is gone after it gets there.
 */
export function blend(p: Spot, s: Spot, t: number, until = 1): Vec2 | undefined {
  if (t > until) return undefined
  const k = until > 0 ? t / until : 1
  return { x: p.X + (s.X - p.X) * k, y: p.Y + (s.Y - p.Y) * k }
}

/** One entity's objects. root goes in the scene. */
export interface EntityView {
  readonly root: Object3D
}

/**
 * One kind of entity's views, by serial, drawn from its timeline. A view is made when its entity shows
 * up, and removed once it ended (blend) or the server stops reporting it. end, if given, says where a
 * state is drawn when not at its own X, Y: a shot that ended on another entity.
 */
export class EntityViews<S extends Placed, V extends EntityView> {
  readonly #parent: Object3D
  readonly #timeline: Timeline<readonly S[]>
  readonly #create: (s: S) => V
  readonly #place: (view: V, s: S, x: number, y: number) => void
  readonly #end: ((s: S) => Spot | undefined) | undefined
  readonly #views = new Map<bigint, V>()
  readonly #seen = new Set<bigint>()

  constructor(
    parent: Object3D,
    timeline: Timeline<readonly S[]>,
    create: (s: S) => V,
    place: (view: V, s: S, x: number, y: number) => void,
    end?: (s: S) => Spot | undefined,
  ) {
    this.#parent = parent
    this.#timeline = timeline
    this.#create = create
    this.#place = place
    this.#end = end
  }

  /** Where the last draw put the entity with serial, or undefined if it didn't draw it. */
  at(serial: bigint): Vec2 | undefined {
    const view = this.#views.get(serial)
    return view && { x: view.root.position.x, y: view.root.position.y }
  }

  /** Places each entity at tick, blended from its older sampled state (if it has one). */
  draw(tick: number | undefined): void {
    this.#seen.clear()
    const sample = tick === undefined ? undefined : this.#timeline.sample(tick)
    if (sample) {
      const { from, to, t } = sample
      const before = new Map(from.map((s): [bigint, S] => [s.Serial, s]))
      for (const s of to) {
        const start = s.PrevX === undefined || s.PrevY === undefined ? undefined : { X: s.PrevX, Y: s.PrevY }
        const at = blend(start ?? before.get(s.Serial) ?? s, this.#end?.(s) ?? s, t, s.Until)
        if (at) this.#place(this.#view(s), s, at.x, at.y)
      }
    }
    for (const [serial, view] of this.#views) {
      if (this.#seen.has(serial)) continue
      this.#parent.remove(view.root)
      this.#views.delete(serial)
    }
  }

  #view(s: S): V {
    this.#seen.add(s.Serial)
    const found = this.#views.get(s.Serial)
    if (found) return found
    const view = this.#create(s)
    this.#views.set(s.Serial, view)
    this.#parent.add(view.root)
    return view
  }
}
