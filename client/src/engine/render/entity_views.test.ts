import { Group, Object3D } from 'three'
import { describe, expect, test } from 'vitest'
import { Timeline } from '../sync/timeline'
import { EntityViews, blend } from './entity_views'
import type { Placed } from './entity_views'

describe('blend', () => {
  test('draws a live entity t of the way from its older state', () => {
    expect(blend({ X: 0, Y: 0 }, { X: 2, Y: 4 }, 0.25)).toEqual({ x: 0.5, y: 1 })
    expect(blend({ X: 0, Y: 0 }, { X: 2, Y: 4 }, 1)).toEqual({ x: 2, y: 4 })
  })

  test('moves one that ended at its own pace to where it ended, then drops it', () => {
    const from = { X: 0, Y: 0 }
    const end = { X: 1, Y: 0 } // reached a quarter of the way through the tick
    expect(blend(from, end, 0.125, 0.25)).toEqual({ x: 0.5, y: 0 })
    expect(blend(from, end, 0.25, 0.25)).toEqual({ x: 1, y: 0 })
    expect(blend(from, end, 0.5, 0.25)).toBeUndefined()
  })

  test('shows one that ended as the tick began only at its end', () => {
    expect(blend({ X: 0, Y: 0 }, { X: 1, Y: 0 }, 0, 0)).toEqual({ x: 1, y: 0 })
    expect(blend({ X: 0, Y: 0 }, { X: 1, Y: 0 }, 0.1, 0)).toBeUndefined()
  })
})

describe('EntityViews', () => {
  test('draws an entity that ended in the newer tick up to its end, then removes its view', () => {
    const shots = new Timeline<Placed[]>()
    shots.add(10, [{ EntityID: 1, X: 0, Y: 0 }])
    shots.add(11, [{ EntityID: 1, X: 1, Y: 0, Until: 0.5 }])
    const parent = new Group()
    const views = new EntityViews(
      parent,
      shots,
      () => ({ root: new Object3D() }),
      (view, _s, x, y) => view.root.position.set(x, y, 0),
    )
    views.draw(10.25)
    expect(parent.children.map((c) => c.position.x)).toEqual([0.5])
    views.draw(10.5)
    expect(parent.children.map((c) => c.position.x)).toEqual([1])
    views.draw(10.75)
    expect(parent.children).toEqual([])
  })
})
