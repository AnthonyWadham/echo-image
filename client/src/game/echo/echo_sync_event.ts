import type { Message } from '../../engine/net/shard_connection'
import type { ShownEffect } from '../player/body_sync_event'

// Mirrors the Go types field for field. ../wire_fixtures.test.ts fails when they drift.

export const EchoSyncEvent: Message<EchoSync> = { name: 'echo_sync' }

/** echo.EchoSync: shards/game/internal/echo/echo_sync_event.go */
export interface EchoSync {
  Tick: bigint // uint64
  Echoes: EchoState[]
}

/**
 * echo.EchoState: what an Echo's row in target_sync lacks. Its row has its owner, team, spot, size, tags
 * and when it died in the tick.
 */
export interface EchoState {
  EntityID: number // uint32: names it in this tick's syncs
  Serial: bigint // uint64: names it across ticks, as its row's
  PlayerID: number // uint32
  Effects: ShownEffect[] // its effect kinds from the next tick on, for their visuals
  HP: number // 0 the tick it dies, when it's sent once more
  MaxHP: number
}
