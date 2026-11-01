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
  Owner: number // uint32: its Body's EntityID
  Team: number // uint8
  X: number
  Y: number
  Radius: number // its collider circle: the hurtbox shots meet
  Tags: bigint // interact.Tags: what its effects make it from the next tick on
  HP: number
  MaxHP: number
  Until: number // the part of the tick it lived: 1, or when it died in it (sent once more, at HP 0)
}
