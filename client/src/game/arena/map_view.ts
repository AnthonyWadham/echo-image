import {
  BufferGeometry,
  ExtrudeGeometry,
  Group,
  LineBasicMaterial,
  LineLoop,
  Mesh,
  Shape,
  Vector2,
  Vector3,
} from 'three'
import type { Object3D } from 'three'
import { lit } from '../../engine/render/color'
import type { Spot } from '../../engine/render/entity_views'
import type { Bounds } from '../../engine/render/stage'

/** A map's shapes in meters, as the server builds them: shards/game/predictor's shapes export. */
export interface MapShapes {
  Solids: Solid[]
  Outlines: Spot[][] // loops around walls
  Edges: Spot[][] // loops around the play area
  Bounds?: Bounds // the box around its shapes and objects: none for an empty map
}

/** One convex piece of wall: level.Solid. */
export type Solid =
  | { Polygon: Spot[] }
  | { Circle: { Center: Spot; Radius: number } }
  | { Capsule: { Center1: Spot; Center2: Spot; Radius: number } }

export const WallHeight = 1 // m
const Lift = 0.01 // m: a line sits this far above what it lines, so it draws on top
const extrude = { depth: WallHeight, bevelEnabled: false, curveSegments: 24 }
const wall = lit([0.42, 0.45, 0.48])
const line = new LineBasicMaterial({ color: 0x9aa6b2 })

/** The live map in grey: solids as blocks WallHeight tall, outlines on their tops, edges on the ground. */
export class MapView {
  readonly #root = new Group()
  readonly #shapes: (id: string) => MapShapes | undefined
  #id: string | undefined
  /** The drawn map's bounds: undefined with none drawn. */
  bounds: Bounds | undefined

  constructor(parent: Object3D, shapes: (id: string) => MapShapes | undefined) {
    this.#shapes = shapes
    parent.add(this.#root)
  }

  /** Draws map id, unless it's drawn already. */
  show(id: string | undefined): void {
    if (id === this.#id) return
    this.#id = id
    for (const child of this.#root.children) {
      if (child instanceof Mesh || child instanceof LineLoop) child.geometry.dispose()
    }
    this.#root.clear()
    this.bounds = undefined
    const m = id === undefined ? undefined : this.#shapes(id)
    if (!m) return
    for (const s of m.Solids) {
      this.#root.add(new Mesh(new ExtrudeGeometry(outline(s), extrude), wall))
    }
    for (const [loops, z] of [
      [m.Outlines, WallHeight + Lift],
      [m.Edges, Lift],
    ] as const) {
      for (const points of loops) {
        const geometry = new BufferGeometry().setFromPoints(points.map((p) => new Vector3(p.X, p.Y, z)))
        this.#root.add(new LineLoop(geometry, line))
      }
    }
    this.bounds = m.Bounds
  }
}

/** A solid's outline, to extrude. */
function outline(s: Solid): Shape {
  if ('Polygon' in s) return new Shape(s.Polygon.map((p) => new Vector2(p.X, p.Y)))
  if ('Circle' in s) {
    const { Center, Radius } = s.Circle
    return new Shape().absarc(Center.X, Center.Y, Radius, 0, 2 * Math.PI, false)
  }
  // Half a circle round each end, joined by the sides.
  const { Center1: a, Center2: b, Radius } = s.Capsule
  const angle = Math.atan2(b.Y - a.Y, b.X - a.X)
  return new Shape()
    .absarc(a.X, a.Y, Radius, angle + Math.PI / 2, angle + (3 * Math.PI) / 2, false)
    .absarc(b.X, b.Y, Radius, angle - Math.PI / 2, angle + Math.PI / 2, false)
}
