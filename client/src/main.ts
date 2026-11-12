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
import { loadPredictor } from './game/player/predictor'
import { ProjectileSyncEvent } from './game/projectile/projectile_sync_event'
import type { ProjectileState } from './game/projectile/projectile_sync_event'
import { newProjectileViews } from './game/projectile/projectile_view'
import { TargetSyncEvent } from './game/track/target_sync_event'
import { showHome } from './game/ui/home'
import { Hud } from './game/ui/hud'

// The DEV-auth player. With none in the URL, the home screen picks one; a second tab with another
// ID takes the other Body.
const playerId = new URLSearchParams(location.search).get('player')
if (playerId) void play(playerId)
else showHome(document.body)

async function play(playerId: string): Promise<void> {
  const hudElement = document.getElementById('hud')
  if (!hudElement) throw new Error('index.html has no #hud')
  // The predictor runs from the first frame, so load it before connecting.
  const predictor = await loadPredictor().catch((error: unknown) => {
    hudElement.textContent = `Loading the predictor failed: ${String(error)}. Reload to retry.`
    throw error
  })

  const net = new ShardConnection(playerId, {
    // Vite's /world proxy. /argus is world.toml's organization: older world CLIs only route
    // /<org>/<project>/<shard>, and newer ones still accept it.
    gatewayUrl: `${location.origin}/world/argus`,
    project: 'echo-image', // world.toml
    shardId: 'game',
  })

  // One timeline per sync event. They start over together.
  const timelines = new Timelines(predictor.interpolationSeconds)
  const player = new LocalPlayer(timelines, newSession(), predictor)
  const echoes = timelines.create<EchoState[]>()
  const abilities = timelines.create<Cooldown[]>()
  const shots = timelines.create<ProjectileState[]>()

  const stage = new Stage(document.body)
  const bodyViews = newBodyViews(stage.scene, player)
  const echoViews = newEchoViews(stage.scene, echoes, player)
  // Shots draw last, so one that ended on a Body or Echo ends where this frame draws it.
  const targetAt = (serial: bigint) => bodyViews.at(serial) ?? echoViews.at(serial)
  const views = [bodyViews, echoViews, newProjectileViews(stage, shots, player, targetAt)]
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
      on(TargetSyncEvent, (s) => player.onTargetSync(s)),
    ])
    .then((error) => {
      if (error) problem = `Subscribing to sync events failed: ${error.message}. Reload to retry.`
    })

  let last: number | undefined
  let drawn: number | undefined // the render tick on screen, which this frame's inputs were made seeing
  function frame(now: number): void {
    requestAnimationFrame(frame)
    const dt = last === undefined ? 0 : (now - last) / 1000
    last = now
    const pointer = controls.pointer
    const held = {
      move: controls.move,
      ground: pointer && stage.groundPoint(pointer),
      takeButtons: () => controls.takeButtons(),
      view: drawn,
    }
    for (const input of player.inputsToSend(dt, held)) net.send(InputCommand, input)
    drawn = timelines.renderTick(dt, player.tickSeconds)
    for (const view of views) view.draw(drawn)
    stage.render()
    hud.draw(net.status, problem)
  }
  requestAnimationFrame(frame)
}
