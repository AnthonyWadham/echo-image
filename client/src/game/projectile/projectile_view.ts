import { Mesh, SphereGeometry } from 'three'
import type { Vec2 } from '../../engine/math/vec2'
import { lit, mix } from '../../engine/render/color'
import type { Rgb } from '../../engine/render/color'
import { EntityViews } from '../../engine/render/entity_views'
import type { Stage } from '../../engine/render/stage'
import type { Timeline } from '../../engine/sync/timeline'
import type { LocalPlayer } from '../player/local_player'
import { TeamLooks } from '../team/team_look'
import { endOn } from './end_on'
import type { TargetEnd } from './end_on'
import type { ProjectileState } from './projectile_sync_event'

const geometry = new SphereGeometry(1, 16, 8) // shared, and scaled to each shot's Radius
const Yellow: Rgb = [1, 0.92, 0.016] // Unity's Color.yellow

/** A shot: a ball of its Radius, its team's colour mixed with yellow. */
interface ProjectileView {
  readonly root: Mesh
}

/**
 * Every shot in flight: others' from their timeline, minus yours, which are drawn where you predict them.
 * One that ended on a Body or Echo ends on it, where targetAt says this frame draws it.
 */
export function newProjectileViews(
  stage: Stage,
  shots: Timeline<ProjectileState[]>,
  player: LocalPlayer,
  targetAt: (serial: bigint) => Vec2 | undefined,
): { draw(tick: number | undefined): void } {
  const looks = new TeamLooks((c) => lit(mix(c, Yellow, 0.5)))
  const end = (e: TargetEnd) => endOn(e, targetAt)
  const others = new EntityViews(
    stage.scene,
    shots,
    (s): ProjectileView => ({ root: new Mesh(geometry, looks.get(s.Team)) }),
    (view, s, x, y) => {
      view.root.material = looks.get(s.Team)
      view.root.position.set(x, y, 0.5)
      view.root.scale.setScalar(s.Radius)
    },
    end,
  )
  // Your shots have no entity until the server confirms them, so they're a pool of balls, one per shot.
  const yours: Mesh[] = []
  return {
    draw(tick) {
      others.draw(tick)
      const drawn = player.drawnShots(end)
      const team = player.team ?? 0
      while (yours.length < drawn.length) {
        const ball = new Mesh(geometry, looks.get(team))
        yours.push(ball)
        stage.scene.add(ball)
      }
      for (const [i, ball] of yours.entries()) {
        const s = drawn[i]
        ball.visible = s !== undefined
        if (!s) continue
        ball.material = looks.get(team)
        ball.position.set(s.X, s.Y, 0.5)
        ball.scale.setScalar(s.Radius)
      }
    },
  }
}
