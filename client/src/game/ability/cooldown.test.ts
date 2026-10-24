import { expect, test } from 'vitest'
import { cooldowns } from './cooldown'

test('takes ReadyTick as a number', () => {
  const s = { Tick: 7n, Abilities: [{ Body: 1, Slot: 0, Ability: 'bolt', ReadyTick: 47n }] }
  expect(cooldowns(s)).toEqual([{ Body: 1, Slot: 0, Ability: 'bolt', ReadyTick: 47 }])
})
