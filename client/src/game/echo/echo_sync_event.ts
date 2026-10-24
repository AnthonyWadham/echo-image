import type { Message } from '../../engine/net/shard_connection'

// Mirrors the Go types field for field. ../wire_fixtures.test.ts fails when they drift.

export const EchoSyncEvent: Message<EchoSync> = { name: 'echo_sync' }

/** echo.EchoSync: shards/game/internal/echo/echo_sync_event.go */
export interface EchoSync {
  Tick: bigint // uint64
  Echoes: EchoState[]
}

/** echo.EchoState */
export interface EchoState {
  EntityID: number // uint32
  PlayerID: number // uint32
  Team: number // uint8
  X: number
  Y: number
  HP: number
  MaxHP: number
}
