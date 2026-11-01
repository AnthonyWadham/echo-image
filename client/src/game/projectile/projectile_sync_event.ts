import type { Message } from '../../engine/net/shard_connection'

// Mirrors the Go types field for field. ../wire_fixtures.test.ts fails when they drift.

export const ProjectileSyncEvent: Message<ProjectileSync> = { name: 'projectile_sync' }

/** projectile.ProjectileSync: shards/game/internal/projectile/projectile_sync_event.go */
export interface ProjectileSync {
  Tick: bigint // uint64
  Projectiles: ProjectileState[]
}

/**
 * projectile.ProjectileState. Until is the part of the tick it flew: 1 in flight, and for an Ended shot
 * when it reached X, Y.
 */
export interface ProjectileState {
  EntityID: number // uint32
  Type: string // shot type ID, for its look
  Caster: number // uint32 EntityID: your Body's shots are drawn from your predictor instead
  Team: number // uint8
  X: number
  Y: number
  Radius: number
  Until: number
  Ended: boolean // it ended this tick at X, Y; the next sync drops it
}
