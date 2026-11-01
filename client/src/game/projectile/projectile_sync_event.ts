import type { Message } from '../../engine/net/shard_connection'

// Mirrors the Go types field for field. ../wire_fixtures.test.ts fails when they drift.

export const ProjectileSyncEvent: Message<ProjectileSync> = { name: 'projectile_sync' }

/** projectile.ProjectileSync: shards/game/internal/projectile/projectile_sync_event.go */
export interface ProjectileSync {
  Tick: bigint // uint64
  Projectiles: ProjectileState[]
}

/**
 * projectile.ProjectileState. It flew this tick from PrevX, PrevY (its launch point on its first tick) to
 * X, Y, and Until is the part of the tick that took: 1 in flight, and for an Ended shot when it got
 * there. An Ended shot that met a Body or Echo (OnTarget) ends on the one with serial TargetSerial,
 * Offset from its center, where each screen draws it (endOn). Your own predicted shots aren't sent to
 * you: your predictor draws them.
 */
export interface ProjectileState {
  EntityID: number // uint32: names it in this tick's syncs
  Serial: bigint // uint64: names it across ticks, as a newer shot can take its EntityID
  Type: string // shot type ID, for its look
  Caster: number // uint32 EntityID: the Body or Echo that cast it
  Team: number // uint8
  PrevX: number
  PrevY: number
  X: number
  Y: number
  Radius: number
  Until: number
  Ended: boolean // it ended this tick at X, Y; the next sync drops it
  OnTarget: boolean
  TargetSerial: bigint // uint64
  OffsetX: number
  OffsetY: number
}
