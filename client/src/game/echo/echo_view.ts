import { CylinderGeometry, Group, Mesh, MeshBasicMaterial, PlaneGeometry } from 'three'
import type { Material, Object3D } from 'three'
import { lit } from '../../engine/render/color'
import { EntityViews } from '../../engine/render/entity_views'
import type { Timeline } from '../../engine/sync/timeline'
import type { LocalPlayer } from '../player/local_player'
import { TeamLooks } from '../team/team_look'
import { EchoKind } from '../track/target_sync_event'
import type { TargetState } from '../track/target_sync_event'
import type { EchoState } from './echo_sync_event'

// Shared by every view, so a view owns nothing to dispose.
const disc = new CylinderGeometry(0.5, 0.5, 0.1, 32)
  .rotateX(Math.PI / 2) // axis along z, lying on the ground
  .translate(0, 0, 0.05)
const bar = new PlaneGeometry(1, 0.12).translate(0.5, 0, 0) // grows right from x = 0
const barBack = new MeshBasicMaterial({ color: 0x6b2a2a, depthTest: false }) // HP lost
const barFill = new MeshBasicMaterial({ color: 0x7ee06a, depthTest: false })

/** An Echo: a disc in its team's colour, darker than a Body's, under an HP bar. */
interface EchoView {
  readonly root: Group
  readonly shape: Mesh
  readonly hp: Mesh
}

function echoView(material: Material): EchoView {
  const shape = new Mesh(disc, material)
  const back = new Mesh(bar, barBack)
  const hp = new Mesh(bar, barFill)
  back.position.set(-0.5, 0.75, 0.2)
  hp.position.set(-0.5, 0.75, 0.2)
  back.renderOrder = 1 // bars draw over everything, the fill over its back
  hp.renderOrder = 2
  return { root: new Group().add(shape, back, hp), shape, hp }
}

/** Every Echo, from its rows, in scene, with its health from the same tick's echo_sync: yours as predicted. */
export function newEchoViews(
  scene: Object3D,
  echoes: Timeline<EchoState[]>,
  player: LocalPlayer,
): EntityViews<TargetState, EchoView> {
  const looks = new TeamLooks(([r, g, b]) => lit([r * 0.6, g * 0.6, b * 0.6]))
  return new EntityViews(
    scene,
    player.targets,
    (r) => echoView(looks.get(r.Team)),
    (view, r, x, y, tick) => {
      const at = player.drawnOwn(r.Serial)
      const e = echoes.sample(tick)?.to.find((s) => s.Serial === r.Serial)
      view.shape.material = looks.get(r.Team)
      view.root.position.set(at?.x ?? x, at?.y ?? y, 0)
      view.hp.scale.setX(e && e.MaxHP > 0 ? Math.min(Math.max(e.HP / e.MaxHP, 0), 1) : 0)
    },
    { shows: (r) => r.Kind === EchoKind },
  )
}
