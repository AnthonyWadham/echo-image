import { Box3, Group, LineLoop, Mesh } from 'three'
import type { Object3D } from 'three'
import { describe, expect, test } from 'vitest'
import { MapView, WallHeight } from './map_view'
import type { MapFile, MapShapes } from './map_view'

// A square wall, a round one and a capsule along x, the square outlined, inside a 20 x 20 m edge: in
// units, 1024 to a meter.
const file: MapFile = {
  solids: [
    { polygon: [[1024, 1024], [2048, 1024], [2048, 2048], [1024, 2048]] },
    { circle: { center: [-3072, 4096], radius: 512 } },
    { capsule: { center1: [3072, -2048], center2: [6144, -2048], radius: 256 } },
  ],
  outlines: [{ points: [[1024, 1024], [2048, 1024], [2048, 2048], [1024, 2048]] }],
  edges: [{ points: [[-10240, -10240], [-10240, 10240], [10240, 10240], [10240, -10240]] }],
}
const shapes: MapShapes = { File: file, Bounds: { min: { x: -10, y: -10 }, max: { x: 10, y: 10 } } }
const maps: Record<string, MapShapes> = {
  walls: shapes,
  open: { ...shapes, File: { ...file, solids: [], outlines: [] } },
}

// The box o fills, as its lowest and highest corners: to 6 places, since three.js keeps 32-bit floats.
const box = (o: Object3D) => {
  const b = new Box3().setFromObject(o)
  return [b.min, b.max].map((v) => v.toArray().map((n) => Math.round(n * 1e6) / 1e6))
}

describe('MapView', () => {
  test('draws each solid as a block WallHeight tall, outlines on their tops and edges on the ground', () => {
    const parent = new Group()
    const view = new MapView(parent, (id) => maps[id])
    view.show('walls')
    const drawn = parent.children[0]?.children ?? []
    expect(drawn.filter((c) => c instanceof Mesh).map(box)).toEqual([
      [[1, 1, 0], [2, 2, WallHeight]],
      [[-3.5, 3.5, 0], [-2.5, 4.5, WallHeight]],
      [[2.75, -2.25, 0], [6.25, -1.75, WallHeight]], // round ends, not past them
    ])
    expect(drawn.filter((c) => c instanceof LineLoop).map(box)).toEqual([
      [[1, 1, WallHeight + 0.01], [2, 2, WallHeight + 0.01]],
      [[-10, -10, 0.01], [10, 10, 0.01]],
    ])
    expect(view.bounds).toBe(shapes.Bounds)
  })

  test('redraws only for another map, freeing the old, and draws nothing for one it lacks', () => {
    const parent = new Group()
    const view = new MapView(parent, (id) => maps[id])
    const drawn = () => parent.children[0]?.children ?? []
    view.show('walls')
    const first = [...drawn()]
    view.show('walls')
    expect(drawn().every((c, i) => c === first[i])).toBe(true)
    let freed = 0
    for (const c of first) {
      if (c instanceof Mesh || c instanceof LineLoop) c.geometry.addEventListener('dispose', () => freed++)
    }
    view.show('open')
    expect(freed).toBe(first.length)
    expect(drawn().map((c) => c.type)).toEqual(['LineLoop'])
    expect(view.bounds).toBe(shapes.Bounds)
    view.show('missing')
    expect(drawn()).toEqual([])
    expect(view.bounds).toBeUndefined()
  })
})
