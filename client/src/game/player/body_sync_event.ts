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

/** player.YouState: everything your predictor starts from. */
export interface YouState {
  Body: number // uint32 EntityID
  InputSession: number // uint32
  InputAck: number // uint32
  MoveSpeed: number
  Champion: string // the champion your Body plays: its kit
  Cooldowns: CooldownEntry[] // when each ability your Body has cast is ready again
  Effects: EffectEntry[] // your Body's timed effects, for the predictor
  EffectsNext: number // uint32: the last serial your Body's effect list gave out
  Shots: ShotState[] // the shots your Body cast that are in flight
}

/** cast.Cooldown: shards/game/internal/ability/cast/cast.go */
export interface CooldownEntry {
  Ability: string
  ReadyTick: bigint // uint64
}

/** status.Entry: shards/game/internal/effect/status/status.go */
export interface EffectEntry {
  Kind: string
  Tags: bigint // interact.Tags
  Pass: bigint // interact.Kind: map kinds the holder moves through
  Stat: number // status.Stat
  Value: number // int32 basis points
  VelX: number // forced velocity, units per second
  VelY: number
  Source: number // uint32 EntityID
  Start: bigint // uint64 tick; active for Start <= tick < End
  End: bigint // uint64 tick
  Serial: number // uint32
}

/** player.ShotState: one of your shots in flight. */
export interface ShotState {
  Type: string // shot type ID: its rules
  X: number
  Y: number
  VelX: number // units per second
  VelY: number
  Radius: number
  Remaining: number // distance left before it expires
}

/** player.BodyState */
export interface BodyState {
  EntityID: number // uint32
  PlayerID: number // uint32
  Nickname: string
  Team: number // uint8
  X: number
  Y: number
  Radius: number // its collider circle
}
