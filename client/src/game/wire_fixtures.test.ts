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
  You: {
    Body: 1,
    InputSession: 4294967295,
    InputAck: 42,
    MoveSpeed: 5,
    Champion: 'starter',
    Cooldowns: [
      { Ability: 'bolt', ReadyTick: 9007199254740993n },
      { Ability: 'dash', ReadyTick: 0n },
    ],
    Effects: [
      {
        Kind: 'slow',
        Tags: 2n,
        Pass: 0n,
        Stat: 1,
        Value: -4000,
        VelX: 0,
        VelY: 0,
        Source: 2,
        Start: 9007199254740993n,
        End: 18446744073709551615n,
        Serial: 1,
      },
      {
        Kind: 'dash',
        Tags: 4n,
        Pass: 1n,
        Stat: 0,
        Value: 0,
        VelX: -12.5,
        VelY: 16.25,
        Source: 1,
        Start: 7n,
        End: 13n,
        Serial: 2,
      },
    ],
    EffectsNext: 4294967295,
    Shots: [
      {
        Type: 'bolt',
        Ability: 'bolt',
        Seq: 4294967295,
        Launch: 1,
        X: -4.25,
        Y: 1.5,
        VelX: 9,
        VelY: -12,
        Radius: 0.15,
        Remaining: 17.75,
      },
    ],
  },
  Bodies: [
    { EntityID: 1, PlayerID: 1, Nickname: 'Player One', Team: 0, X: 1.5, Y: -2, Radius: 0.5, Tags: 0n },
    { EntityID: 2, PlayerID: 2, Nickname: 'Échø ✦', Team: 1, X: -0.25, Y: 3.75, Radius: 0.75, Tags: 3n },
  ],
}
const echoSync: EchoSync = {
  Tick: 7n,
  Echoes: [
    { EntityID: 3, PlayerID: 1, Owner: 1, Team: 0, X: -4.5, Y: 0.5, Radius: 0.5, Tags: 8n, HP: 62.5, MaxHP: 100,
      Until: 1 },
    { EntityID: 4, PlayerID: 2, Owner: 2, Team: 1, X: 3, Y: -1.25, Radius: 0.5, Tags: 0n, HP: 0, MaxHP: 100,
      Until: 0.625 },
  ],
}
const abilitySync: AbilitySync = {
  Tick: 7n,
  Abilities: [{ Body: 1, Slot: 0, Ability: 'bolt', ReadyTick: 18446744073709551615n }],
}
const projectileSync: ProjectileSync = {
  Tick: 8n,
  Projectiles: [
    { EntityID: 9, Type: 'bolt', Caster: 4294967295, Team: 1, X: 0.125, Y: -7.5, Radius: 0.15, Until: 1, Ended: false },
    { EntityID: 10, Type: 'bolt', Caster: 3, Team: 0, X: 2.5, Y: 1, Radius: 0.15, Until: 0.375, Ended: true },
  ],
}
const projectileSyncEmpty: ProjectileSync = { Tick: 9n, Projectiles: [] }
const input: Input = {
  Session: 305419896,
  Frames: [
    { Seq: 1, MoveX: 1, MoveY: 0, AimX: 0, AimY: 0, Buttons: 0, ViewTick: 0, Claims: [] },
    {
      Seq: 2,
      MoveX: 0.7071067811865476,
      MoveY: -0.7071067811865476,
      AimX: 3.5,
      AimY: -1.25,
      Buttons: 1,
      ViewTick: 41.375,
      Claims: [
        { Seq: 1, Ability: 'bolt', Launch: 1, Target: 4294967295 },
        { Seq: 2, Ability: 'nova', Launch: 0, Target: 3 },
      ],
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
