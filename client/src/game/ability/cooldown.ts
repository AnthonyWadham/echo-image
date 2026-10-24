import { toTick } from '../../engine/sync/timeline'
import type { AbilityState, AbilitySync } from './ability_sync_event'

/** A kit slot's cooldown: an AbilityState with ReadyTick as a number. */
export type Cooldown = Omit<AbilityState, 'ReadyTick'> & { ReadyTick: number }

export function cooldowns(s: AbilitySync): Cooldown[] {
  return s.Abilities.map((a) => ({ ...a, ReadyTick: toTick(a.ReadyTick) }))
}
