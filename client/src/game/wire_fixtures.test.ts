import { readFileSync, writeFileSync } from 'node:fs'
import { decodePayload, encodePayload } from '@argus-labs/sdk'
import { expect, test } from 'vitest'
import type { AbilitySync } from './ability/ability_sync_event'
import type { EchoSync } from './echo/echo_sync_event'
import type { BodySync } from './player/body_sync_event'
import type { Input } from './player/input_command'
import type { ProjectileSync } from './projectile/projectile_sync_event'

// Golden payloads from TestWireFixtures (shards/game/main_test.go), built from the same samples.
// They pin each *_event.ts and input_command.ts type to its Go struct: a Go uint64 decodes as a
// bigint, every other number as a number.
const dir = new URL('../../../shards/game/testdata/wire/', import.meta.url)
const read = (name: string) => new Uint8Array(readFileSync(new URL(`${name}.msgpack`, dir)))

const bodySync: BodySync = {
  Tick: 9007199254740993n,
  TickSeconds: 0.05,
  You: { Body: 1, InputSession: 4294967295, InputAck: 42, MoveSpeed: 5 },
  Bodies: [
    { EntityID: 1, PlayerID: 1, Nickname: 'Player One', Team: 0, X: 1.5, Y: -2 },
    { EntityID: 2, PlayerID: 2, Nickname: 'Échø ✦', Team: 1, X: -0.25, Y: 3.75 },
  ],
}
const echoSync: EchoSync = {
  Tick: 7n,
  Echoes: [{ EntityID: 3, PlayerID: 1, Team: 0, X: -4.5, Y: 0.5, HP: 62.5, MaxHP: 100 }],
}
const abilitySync: AbilitySync = {
  Tick: 7n,
  Abilities: [{ Body: 1, Slot: 0, Ability: 'bolt', ReadyTick: 18446744073709551615n }],
}
const projectileSync: ProjectileSync = {
  Tick: 8n,
  Projectiles: [{ EntityID: 9, Team: 1, X: 0.125, Y: -7.5, Radius: 0.15 }],
}
const projectileSyncEmpty: ProjectileSync = { Tick: 9n, Projectiles: [] }
const input: Input = {
  Session: 305419896,
  Frames: [
    { Seq: 1, MoveX: 1, MoveY: 0, AimX: 0, AimY: 0, Buttons: 0 },
    {
      Seq: 2,
      MoveX: 0.7071067811865476,
      MoveY: -0.7071067811865476,
      AimX: 3.5,
      AimY: -1.25,
      Buttons: 1,
    },
  ],
}

test.each([
  ['body_sync', bodySync],
  ['echo_sync', echoSync],
  ['ability_sync', abilitySync],
  ['projectile_sync', projectileSync],
  ['projectile_sync_empty', projectileSyncEmpty],
  ['input', input],
])('%s.msgpack decodes to its client type', (name, want) => {
  expect(decodePayload(read(name))).toStrictEqual(want)
})

// TestWireFixtures/input_client decodes these bytes on the server. UPDATE_FIXTURES=1 rewrites them.
test('Input encodes as input_client.msgpack', () => {
  const bytes = encodePayload(input)
  if (process.env.UPDATE_FIXTURES) writeFileSync(new URL('input_client.msgpack', dir), bytes)
  else expect(bytes).toStrictEqual(read('input_client'))
})
