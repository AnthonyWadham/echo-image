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
import type { Bounds } from '../../engine/render/stage'

/** A map as the server decoded it: shards/game/predictor's shapes export. */
export interface MapShapes {
  File: MapFile
  Bounds?: Bounds // the box around its shapes and objects, in meters: none for an empty map
}

/** A map file, as pkg/level writes it: points in whole units, Unit to a meter. */
export interface MapFile {
  solids: Solid[]
  outlines: Loop[] // around walls
  edges: Loop[] // around the play area
}

type Units = [number, number]
type Loop = { points: Units[] }

/** One convex piece of wall: level.Solid. */
export type Solid =
  | { polygon: Units[] }
  | { circle: { center: Units; radius: number } }
  | { capsule: { center1: Units; center2: Units; radius: number } }

const Unit = 1024 // units a meter: pkg/level's Unit
const meters = ([x, y]: Units) => new Vector2(x / Unit, y / Unit)

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
    for (const s of m.File.solids) {
      this.#root.add(new Mesh(new ExtrudeGeometry(outline(s), extrude), wall))
    }
    for (const [loops, z] of [
      [m.File.outlines, WallHeight + Lift],
      [m.File.edges, Lift],
    ] as const) {
      for (const { points } of loops) {
        const geometry = new BufferGeometry().setFromPoints(points.map(([x, y]) => new Vector3(x / Unit, y / Unit, z)))
        this.#root.add(new LineLoop(geometry, line))
      }
    }
    this.bounds = m.Bounds
  }
}

/** A solid's outline, to extrude. */
function outline(s: Solid): Shape {
  if ('polygon' in s) return new Shape(s.polygon.map(meters))
  if ('circle' in s) {
    const { x, y } = meters(s.circle.center)
    return new Shape().absarc(x, y, s.circle.radius / Unit, 0, 2 * Math.PI, false)
  }
  // Half a circle round each end, joined by the sides.
  const [a, b, radius] = [meters(s.capsule.center1), meters(s.capsule.center2), s.capsule.radius / Unit]
  const angle = Math.atan2(b.y - a.y, b.x - a.x)
  return new Shape()
    .absarc(a.x, a.y, radius, angle + Math.PI / 2, angle + (3 * Math.PI) / 2, false)
    .absarc(b.x, b.y, radius, angle - Math.PI / 2, angle + Math.PI / 2, false)
}
