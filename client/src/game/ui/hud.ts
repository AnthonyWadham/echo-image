import { Code } from '@argus-labs/sdk'
import type { ShardStatus } from '@argus-labs/sdk'
import type { Timeline } from '../../engine/sync/timeline'
import type { Cooldown } from '../ability/cooldown'
import type { EchoState } from '../echo/echo_sync_event'
import type { LocalPlayer } from '../player/local_player'

const Help = 'WASD / arrows move, mouse aims. Left click casts your first ability, Space your second.'

/** What the HUD shows. */
export interface HudSources {
  player: LocalPlayer
  echoes: Timeline<EchoState[]>
  abilities: Timeline<Cooldown[]>
}

/** The text overlay. Its DOM is only touched when the text changes. */
export class Hud {
  readonly #el: HTMLElement
  readonly #playerId: string
  readonly #from: HudSources
  #text = ''

  constructor(el: HTMLElement, playerId: string, from: HudSources) {
    this.#el = el
    this.#playerId = playerId
    this.#from = from
  }

  draw(status: ShardStatus, problem: string | undefined): void {
    const lines = this.#lines(status)
    const { player } = this.#from
    if (player.otherSim !== undefined) {
      lines.unshift(
        `This page runs game build ${player.sim}, the server ${player.otherSim}. ` +
          'Reload the page (in dev, restart bun run dev first).',
      )
    }
    if (problem) lines.unshift(problem)
    const text = lines.join('\n')
    if (text === this.#text) return
    this.#text = text
    this.#el.textContent = text
  }

  #lines(status: ShardStatus): string[] {
    const { player, echoes, abilities } = this.#from
    const lines = [this.#status(status), `States received: ${player.received}`]
    for (const e of echoes.newest ?? []) {
      const hp = `${Math.round(e.HP)}/${Math.round(e.MaxHP)} HP`
      lines.push(`Echo of P${e.PlayerID} (team ${e.Team}): ${hp}`)
    }
    // Your kit is predicted, so a cast starts its countdown at once.
    const kit = player.kit
    if (kit) {
      for (const s of kit.Slots) lines.push(`Your ${s.Ability}: ${this.#countdown(s.ReadyTick, kit.Tick)}`)
    }
    // The server sends only your team's cooldowns.
    const now = abilities.newestTick
    for (const a of abilities.newest ?? []) {
      if (a.Body === player.you) continue // yours is the kit above
      const owner = player.bodies.newest?.find((b) => b.EntityID === a.Body)
      lines.push(`${owner?.Nickname ?? `Body ${a.Body}`}'s ${a.Ability}: ${this.#countdown(a.ReadyTick, now)}`)
    }
    if (player.playing) {
      const n = player.input.unacked.length
      lines.push(`Inputs in flight: ${n} (~${Math.round(n * player.tickSeconds * 1000)} ms)`)
      const { replays, corrections } = player.prediction
      lines.push(`Prediction: ${replays} replays, ${corrections} corrections`)
      const h = player.inputHealth
      if (h) {
        lines.push(
          `Server input: ran dry ${h.Coasts}, frames dropped ${h.DroppedFrames}, ` +
            `Inputs too big ${h.OversizedInputs}, views floored ${h.FlooredViews}`,
        )
      }
    } else if (status.state === 'connected') {
      lines.push('Waiting for the server to give you a Body…')
    }
    const other = this.#playerId === 'dev-player-2' ? 'dev-player-1' : 'dev-player-2'
    lines.push(Help, `Second player: open ?player=${other} in another tab to take the other Body.`)
    return lines
  }

  // 'ready', or the seconds from now until readyTick.
  #countdown(readyTick: number, now: number): string {
    const left = (readyTick - now) * this.#from.player.tickSeconds
    return readyTick <= now ? 'ready' : `${left.toFixed(1)} s`
  }

  #status({ state, error }: ShardStatus): string {
    const id = this.#playerId
    if (state === 'connected') {
      const me = this.#from.player.me
      const body = me ? `, playing ${me.Nickname} (Body ${me.EntityID}, team ${me.Team})` : ''
      return `Connected as ${id}${body}`
    }
    if (error?.code === Code.Aborted) {
      return `SUPERSEDED: ${id} is playing in another tab or window. Reload this tab to play here.`
    }
    if (state === 'connecting') return error ? `Reconnecting: ${error.message}` : 'Connecting…'
    return error ? `Stopped: ${error.message}` : 'Not connected'
  }
}
