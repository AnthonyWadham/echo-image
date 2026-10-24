import { Code } from '@argus-labs/sdk'
import type { ShardStatus } from '@argus-labs/sdk'
import type { Timeline } from '../../engine/sync/timeline'
import type { Cooldown } from '../ability/cooldown'
import type { EchoState } from '../echo/echo_sync_event'
import type { LocalPlayer } from '../player/local_player'

const Help = 'WASD / arrows move, mouse aims, hold left click to fire.'

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
    // Only your team's cooldowns are sent to you.
    const now = abilities.newestTick
    for (const a of abilities.newest ?? []) {
      const owner = player.bodies.newest?.find((b) => b.EntityID === a.Body)
      const who = a.Body === player.you ? 'Your' : `${owner?.Nickname ?? `Body ${a.Body}`}'s`
      const left = (a.ReadyTick - now) * player.tickSeconds
      lines.push(`${who} ${a.Ability}: ${a.ReadyTick <= now ? 'ready' : `${left.toFixed(1)} s`}`)
    }
    if (player.playing) {
      const n = player.input.unacked.length
      lines.push(`Inputs in flight: ${n} (~${Math.round(n * player.tickSeconds * 1000)} ms)`)
    } else if (status.state === 'connected') {
      lines.push('Waiting for the server to give you a Body…')
    }
    const other = this.#playerId === 'dev-player-2' ? 'dev-player-1' : 'dev-player-2'
    lines.push(Help, `Second player: open ?player=${other} in another tab to take the other Body.`)
    return lines
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
