import { Mesh, SphereGeometry } from 'three'
import type { Object3D } from 'three'
import { White, lit, mix } from '../../engine/render/color'
import { EntityViews } from '../../engine/render/entity_views'
import { TeamLooks } from '../team/team_look'
import { BodyKind } from '../history/target_sync_event'
import type { TargetState } from '../history/target_sync_event'
import type { LocalPlayer } from './local_player'

const geometry = new SphereGeometry(1, 32, 16) // shared, and scaled to each Body's Radius

/** A Body: a ball of its Radius in its team's colour, resting on the ground. */
interface BodyView {
  readonly root: Mesh
}

/** Every Body, from its rows, in scene: yours lighter and where you've predicted it. */
export function newBodyViews(scene: Object3D, player: LocalPlayer): EntityViews<TargetState, BodyView> {
  const looks = new TeamLooks((c) => ({ body: lit(c), you: lit(mix(c, White, 0.5)) }))
  const material = (b: TargetState) => {
    const look = looks.get(b.Team)
    return player.isYou(b.Serial) ? look.you : look.body
  }
  return new EntityViews(
    scene,
    player.targets,
    (b): BodyView => ({ root: new Mesh(geometry, material(b)) }),
    (view, b, x, y) => {
      const at = player.drawnOwn(b.Serial)
      view.root.material = material(b)
      view.root.position.set(at?.x ?? x, at?.y ?? y, b.Radius)
      view.root.scale.setScalar(b.Radius)
    },
    { shows: (r) => r.Kind === BodyKind },
  )
}
