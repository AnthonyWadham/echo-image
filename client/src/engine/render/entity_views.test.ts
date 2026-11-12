import { readFileSync } from 'node:fs'
import { Group, Object3D } from 'three'
import { describe, expect, test } from 'vitest'
import { Timeline } from '../sync/timeline'
import { EntityViews, blend } from './entity_views'
import type { Placed } from './entity_views'

// TestViewFixture's (shards/game/test/wire_test.go): targets over a few ticks of syncs, and where the server's
// flight.RowTicks and AtView have each at views across them, by serial. Your shots meet them there. It's
// JSON, so serials come as numbers.
interface ViewFixture {
  Syncs: { Tick: number; Targets: (Omit<Placed, 'Serial'> & { Serial: number })[] }[]
  Views: { View: number; Drawn: { Serial: number; X: number; Y: number }[] }[]
}
const viewFixture = new URL('../../../../shards/game/test/testdata/view.json', import.meta.url)

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
    shots.add(10, [{ Serial: 1n, X: 0, Y: 0 }])
    shots.add(11, [{ Serial: 1n, X: 1, Y: 0, Until: 0.5 }])
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

  test('draws an end where end puts it, and says where it drew each entity', () => {
    const shots = new Timeline<Placed[]>()
    shots.add(10, [{ Serial: 1n, X: 0, Y: 0 }])
    shots.add(11, [{ Serial: 1n, X: 1, Y: 0, Until: 0.5 }])
    const views = new EntityViews(
      new Group(),
      shots,
      () => ({ root: new Object3D() }),
      (view, _s, x, y) => view.root.position.set(x, y, 0),
      { end: (s) => (s.Until === undefined ? undefined : { X: 2, Y: 2 }) }, // where this screen draws what it ended on
    )
    views.draw(10.25)
    expect(views.at(1n)).toEqual({ x: 1, y: 1 }) // half way to its end at (2, 2)
    expect(views.at(2n)).toBeUndefined()
    views.draw(10.75)
    expect(views.at(1n)).toBeUndefined()
  })

  test('draws a state from where it says it began, not from an older state', () => {
    const shots = new Timeline<Placed[]>()
    shots.add(10, [{ Serial: 1n, X: 5, Y: 5 }])
    shots.add(11, [
      { Serial: 1n, PrevX: 0, PrevY: 0, X: 1, Y: 0 }, // it began the tick at (0, 0): it bounced, say
      { Serial: 2n, PrevX: 0, PrevY: 1, X: 1, Y: 1 }, // one in its first tick
    ])
    const views = new EntityViews(
      new Group(),
      shots,
      () => ({ root: new Object3D() }),
      (view, _s, x, y) => view.root.position.set(x, y, 0),
    )
    views.draw(10.5)
    expect([views.at(1n), views.at(2n)]).toEqual([
      { x: 0.5, y: 0 },
      { x: 0.5, y: 1 },
    ])
  })

  test("gives a new serial its own view, never another's state", () => {
    const states = new Timeline<Placed[]>()
    states.add(9, [{ Serial: 1n, X: 0, Y: 0 }])
    states.add(10, [{ Serial: 1n, X: 0, Y: 0, Until: 0.5 }]) // it ends
    states.add(11, [{ Serial: 2n, X: 4, Y: 0 }]) // a new one, which can have its EntityID
    const made: bigint[] = []
    const views = new EntityViews(
      new Group(),
      states,
      (s) => {
        made.push(s.Serial)
        return { root: new Object3D() }
      },
      (view, _s, x, y) => view.root.position.set(x, y, 0),
    )
    views.draw(9.5)
    views.draw(10.5)
    expect([views.at(1n), views.at(2n)]).toEqual([undefined, { x: 4, y: 0 }])
    expect(made).toEqual([1n, 2n])
  })

  test('draws each target where the server has it at each view, to the bit', () => {
    const fixture = JSON.parse(readFileSync(viewFixture, 'utf8')) as ViewFixture
    const targets = new Timeline<Placed[]>()
    for (const s of fixture.Syncs) targets.add(s.Tick, s.Targets.map((t) => ({ ...t, Serial: BigInt(t.Serial) })))
    const views = new EntityViews(
      new Group(),
      targets,
      () => ({ root: new Object3D() }),
      (view, _s, x, y) => view.root.position.set(x, y, 0),
    )
    const serials = [...new Set(fixture.Syncs.flatMap((s) => s.Targets.map((t) => t.Serial)))].sort((a, b) => a - b)
    expect(fixture.Views.filter((v) => v.Drawn.length > 0).length).toBeGreaterThan(5)
    for (const v of fixture.Views) {
      views.draw(v.View)
      const drawn = serials.flatMap((serial) => {
        const at = views.at(BigInt(serial))
        return at ? [{ Serial: serial, X: at.x, Y: at.y }] : []
      })
      expect(drawn, `view ${v.View}`).toStrictEqual(v.Drawn)
    }
  })
})
