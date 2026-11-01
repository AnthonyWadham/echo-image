import type { Message } from '../../engine/net/shard_connection'

// Mirrors the Go types field for field. ../wire_fixtures.test.ts fails when they drift.

export const TargetSyncEvent: Message<TargetSync> = { name: 'target_sync' }

/** interact.Body and interact.Echo: the kinds of row. */
export const BodyKind = 2n
export const EchoKind = 4n

/**
 * projectile.TargetSync: shards/game/internal/projectile/target_sync_event.go. Screens draw every Body and
 * Echo from its rows, and your predictor places them by it.
 */
export interface TargetSync {
  Tick: bigint // uint64
  Targets: TargetState[]
}

/** projectile.TargetState: a target's row this tick, the record the server's shots meet it by. */
export interface TargetState {
  EntityID: number // uint32: names it in this tick's syncs
  Serial: bigint // uint64: names it across ticks, as a newer entity can take its EntityID
  Kind: bigint // interact.Kind: a Body or an Echo
  Owner: number // uint32: its owner's Body; a Body owns itself
  Team: number // uint8
  X: number
  Y: number
  Radius: number
  Tags: bigint // interact.Tags: what its effects make it from the next tick on
  Until: number // the part of the tick it lived: 1, or when it died in it
}
