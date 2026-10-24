import { Mesh, SphereGeometry } from 'three'
import { White, lit, mix } from '../../engine/render/color'
import { EntityViews } from '../../engine/render/entity_views'
import type { Stage } from '../../engine/render/stage'
import { TeamLooks } from '../team/team_look'
import type { BodyState } from './body_sync_event'
import type { LocalPlayer } from './local_player'

const geometry = new SphereGeometry(0.5, 32, 16) // shared, so a view owns nothing to dispose

/** A Body: a ball in its team's colour. */
interface BodyView {
  readonly root: Mesh
}

/** Every Body, yours lighter. */
export function newBodyViews(stage: Stage, player: LocalPlayer): EntityViews<BodyState, BodyView> {
  const looks = new TeamLooks((c) => ({ body: lit(c), you: lit(mix(c, White, 0.5)) }))
  const material = (b: BodyState) => {
    const look = looks.get(b.Team)
    return b.EntityID === player.you ? look.you : look.body
  }
  return new EntityViews(
    stage.scene,
    player.bodies,
    (b): BodyView => ({ root: new Mesh(geometry, material(b)) }),
    (view, b, x, y) => {
      view.root.material = material(b)
      view.root.position.set(x, y, 0.5)
    },
  )
}
