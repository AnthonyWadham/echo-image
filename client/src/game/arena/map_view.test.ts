import { Box3, Group, LineLoop, Mesh } from 'three'
import type { Object3D } from 'three'
import { describe, expect, test } from 'vitest'
import { MapView, WallHeight, bounds } from './map_view'
import type { MapShapes } from './map_view'

// A square wall, a round one and a capsule along x, the square outlined, inside a 20 x 20 m edge.
const shapes: MapShapes = {
  Solids: [
    { Polygon: [{ X: 1, Y: 1 }, { X: 2, Y: 1 }, { X: 2, Y: 2 }, { X: 1, Y: 2 }] },
    { Circle: { Center: { X: -3, Y: 4 }, Radius: 0.5 } },
    { Capsule: { Center1: { X: 3, Y: -2 }, Center2: { X: 6, Y: -2 }, Radius: 0.25 } },
  ],
  Outlines: [[{ X: 1, Y: 1 }, { X: 2, Y: 1 }, { X: 2, Y: 2 }, { X: 1, Y: 2 }]],
  Edges: [[{ X: -10, Y: -10 }, { X: -10, Y: 10 }, { X: 10, Y: 10 }, { X: 10, Y: -10 }]],
}
const maps: Record<string, MapShapes> = { walls: shapes, open: { ...shapes, Solids: [], Outlines: [] } }

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
    expect(view.bounds).toEqual(bounds(shapes))
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
    expect(view.bounds).toEqual({ min: { x: -10, y: -10 }, max: { x: 10, y: 10 } })
    view.show('missing')
    expect(drawn()).toEqual([])
    expect(view.bounds).toBeUndefined()
  })
})

describe('bounds', () => {
  test("is the box around every shape, a round one's radius included", () => {
    expect(bounds({ ...shapes, Edges: [] })).toEqual({ min: { x: -3.5, y: -2.25 }, max: { x: 6.25, y: 4.5 } })
    expect(bounds(shapes)).toEqual({ min: { x: -10, y: -10 }, max: { x: 10, y: 10 } })
    expect(bounds({ Solids: [], Outlines: [], Edges: [] })).toBeUndefined()
  })
})
