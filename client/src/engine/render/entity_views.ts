import type { Object3D } from 'three'
import type { Timeline } from '../sync/timeline'

/** What a view needs of an entity's state. */
export interface Placed {
  EntityID: number
  X: number
  Y: number
}

/** One entity's objects. root goes in the scene. */
export interface EntityView {
  readonly root: Object3D
}

/**
 * One kind of entity's views, by EntityID, drawn from its timeline. A view is made when its entity
 * shows up and removed once the server stops reporting it.
 */
export class EntityViews<S extends Placed, V extends EntityView> {
  readonly #parent: Object3D
  readonly #timeline: Timeline<readonly S[]>
  readonly #create: (s: S) => V
  readonly #place: (view: V, s: S, x: number, y: number) => void
  readonly #views = new Map<number, V>()
  readonly #seen = new Set<number>()

  constructor(
    parent: Object3D,
    timeline: Timeline<readonly S[]>,
    create: (s: S) => V,
    place: (view: V, s: S, x: number, y: number) => void,
  ) {
    this.#parent = parent
    this.#timeline = timeline
    this.#create = create
    this.#place = place
  }

  /** Places each entity at tick, blended from its older sampled state (if it has one). */
  draw(tick: number | undefined): void {
    this.#seen.clear()
    const sample = tick === undefined ? undefined : this.#timeline.sample(tick)
    if (sample) {
      const { from, to, t } = sample
      const before = new Map(from.map((s): [number, S] => [s.EntityID, s]))
      for (const s of to) {
        const p = before.get(s.EntityID) ?? s
        this.#place(this.#view(s), s, p.X + (s.X - p.X) * t, p.Y + (s.Y - p.Y) * t)
      }
    }
    for (const [id, view] of this.#views) {
      if (this.#seen.has(id)) continue
      this.#parent.remove(view.root)
      this.#views.delete(id)
    }
  }

  #view(s: S): V {
    this.#seen.add(s.EntityID)
    const found = this.#views.get(s.EntityID)
    if (found) return found
    const view = this.#create(s)
    this.#views.set(s.EntityID, view)
    this.#parent.add(view.root)
    return view
  }
}
