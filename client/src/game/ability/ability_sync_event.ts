import type { Message } from '../../engine/net/shard_connection'

// Mirrors the Go types field for field. ../wire_fixtures.test.ts fails when they drift.

export const AbilitySyncEvent: Message<AbilitySync> = { name: 'ability_sync' }

/** ability.AbilitySync: shards/game/internal/ability/ability_sync_event.go */
export interface AbilitySync {
  Tick: bigint // uint64
  Abilities: AbilityState[]
}

/** ability.AbilityState */
export interface AbilityState {
  Body: number // uint32 EntityID
  Slot: number // uint32
  Ability: string
  ReadyTick: bigint // uint64
}
