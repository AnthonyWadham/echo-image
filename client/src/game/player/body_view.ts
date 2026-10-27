import { Mesh, SphereGeometry } from 'three'
import { White, lit, mix } from '../../engine/render/color'
import { EntityViews } from '../../engine/render/entity_views'
import type { Stage } from '../../engine/render/stage'
import { TeamLooks } from '../team/team_look'
import type { BodyState } from './body_sync_event'
import type { LocalPlayer } from './local_player'

const geometry = new SphereGeometry(1, 32, 16) // shared, and scaled to each Body's Radius

/** A Body: a ball of its Radius in its team's colour, resting on the ground. */
interface BodyView {
  readonly root: Mesh
}

/** Every Body, yours lighter and where you've predicted it. */
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
      const at = b.EntityID === player.you ? player.drawn : undefined
      view.root.material = material(b)
      view.root.position.set(at?.x ?? x, at?.y ?? y, b.Radius)
      view.root.scale.setScalar(b.Radius)
    },
  )
}
