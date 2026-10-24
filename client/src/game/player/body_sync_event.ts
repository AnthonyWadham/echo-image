import type { Message } from '../../engine/net/shard_connection'

// Mirrors the Go types field for field. ../wire_fixtures.test.ts fails when they drift.

export const BodySyncEvent: Message<BodySync> = { name: 'body_sync' }

/** player.BodySync: shards/game/internal/player/body_sync_event.go */
export interface BodySync {
  Tick: bigint // uint64
  TickSeconds: number
  You: YouState
  Bodies: BodyState[]
}

/** player.YouState */
export interface YouState {
  Body: number // uint32 EntityID
  InputSession: number // uint32
  InputAck: number // uint32
  MoveSpeed: number
}

/** player.BodyState */
export interface BodyState {
  EntityID: number // uint32
  PlayerID: number // uint32
  Nickname: string
  Team: number // uint8
  X: number
  Y: number
}
