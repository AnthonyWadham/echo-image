import { Controls } from './engine/input/controls'
import { ShardConnection, on } from './engine/net/shard_connection'
import { Stage } from './engine/render/stage'
import { Timelines } from './engine/sync/timeline'
import { AbilitySyncEvent } from './game/ability/ability_sync_event'
import { cooldowns } from './game/ability/cooldown'
import type { Cooldown } from './game/ability/cooldown'
import { EchoSyncEvent } from './game/echo/echo_sync_event'
import type { EchoState } from './game/echo/echo_sync_event'
import { newEchoViews } from './game/echo/echo_view'
import { BodySyncEvent } from './game/player/body_sync_event'
import { newBodyViews } from './game/player/body_view'
import { InputCommand } from './game/player/input_command'
import { newSession } from './game/player/input_stream'
import { LocalPlayer } from './game/player/local_player'
import { ProjectileSyncEvent } from './game/projectile/projectile_sync_event'
import type { ProjectileState } from './game/projectile/projectile_sync_event'
import { newProjectileViews } from './game/projectile/projectile_view'
import { Hud } from './game/ui/hud'

// The DEV-auth player. A second tab with another ID takes the other Body.
const playerId = new URLSearchParams(location.search).get('player') || 'dev-player-1'
const hudElement = document.getElementById('hud')
if (!hudElement) throw new Error('index.html has no #hud')

const net = new ShardConnection(playerId, {
  // Vite's /world proxy. /argus is world.toml's organization: older world CLIs only route
  // /<org>/<project>/<shard>, and newer ones still accept it.
  gatewayUrl: `${location.origin}/world/argus`,
  project: 'echo-image', // world.toml
  shardId: 'game',
})

// One timeline per sync event. They start over together.
const timelines = new Timelines()
const player = new LocalPlayer(timelines, newSession())
const echoes = timelines.create<EchoState[]>()
const abilities = timelines.create<Cooldown[]>()
const shots = timelines.create<ProjectileState[]>()

const stage = new Stage(document.body)
const views = [
  newBodyViews(stage, player),
  newEchoViews(stage, echoes),
  newProjectileViews(stage, shots),
]
const controls = new Controls(stage.canvas)
const hud = new Hud(hudElement, playerId, { player, echoes, abilities })
let problem: string | undefined

net.onStatus((status) => {
  const join = player.setConnected(status.state === 'connected', controls.move)
  if (join) net.send(InputCommand, join)
})
void net
  .subscribe([
    on(BodySyncEvent, (s) => player.onBodySync(s)),
    on(EchoSyncEvent, (s) => timelines.add(echoes, s.Tick, s.Echoes)),
    on(AbilitySyncEvent, (s) => timelines.add(abilities, s.Tick, cooldowns(s))),
    on(ProjectileSyncEvent, (s) => timelines.add(shots, s.Tick, s.Projectiles)),
  ])
  .then((error) => {
    if (error) problem = `Subscribing to sync events failed: ${error.message}. Reload to retry.`
  })

let last: number | undefined
function frame(now: number): void {
  requestAnimationFrame(frame)
  const dt = last === undefined ? 0 : (now - last) / 1000
  last = now
  const pointer = controls.pointer
  const held = {
    move: controls.move,
    ground: pointer && stage.groundPoint(pointer),
    buttons: controls.buttons,
  }
  for (const input of player.inputsToSend(dt, held)) net.send(InputCommand, input)
  const tick = timelines.renderTick(dt, player.bodies, player.tickSeconds)
  for (const view of views) view.draw(tick)
  stage.render()
  hud.draw(net.status, problem)
}
requestAnimationFrame(frame)
