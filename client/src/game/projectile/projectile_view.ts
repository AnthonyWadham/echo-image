import { Mesh, SphereGeometry } from 'three'
import { lit, mix } from '../../engine/render/color'
import type { Rgb } from '../../engine/render/color'
import { EntityViews } from '../../engine/render/entity_views'
import type { Stage } from '../../engine/render/stage'
import type { Timeline } from '../../engine/sync/timeline'
import { TeamLooks } from '../team/team_look'
import type { ProjectileState } from './projectile_sync_event'

const geometry = new SphereGeometry(1, 16, 8) // shared, and scaled to each shot's Radius
const Yellow: Rgb = [1, 0.92, 0.016] // Unity's Color.yellow

/** A shot: a ball of its Radius, its team's colour mixed with yellow. */
interface ProjectileView {
  readonly root: Mesh
}

/** Every shot in flight. */
export function newProjectileViews(
  stage: Stage,
  shots: Timeline<ProjectileState[]>,
): EntityViews<ProjectileState, ProjectileView> {
  const looks = new TeamLooks((c) => lit(mix(c, Yellow, 0.5)))
  return new EntityViews(
    stage.scene,
    shots,
    (s): ProjectileView => ({ root: new Mesh(geometry, looks.get(s.Team)) }),
    (view, s, x, y) => {
      view.root.material = looks.get(s.Team)
      view.root.position.set(x, y, 0.5)
      view.root.scale.setScalar(s.Radius)
    },
  )
}
