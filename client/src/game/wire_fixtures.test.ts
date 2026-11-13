import { readFileSync, writeFileSync } from 'node:fs'
import { decodePayload, encodePayload } from '@argus-labs/sdk'
import { expect, test } from 'vitest'
import type { AbilitySync } from './ability/ability_sync_event'
import type { EchoSync } from './echo/echo_sync_event'
import type { BodySync } from './player/body_sync_event'
import type { Input } from './player/input_command'
import type { ProjectileSync } from './projectile/projectile_sync_event'
import { BodyKind, EchoKind, type TargetSync } from './history/target_sync_event'

// Golden payloads from TestWireFixtures (shards/game/test/wire_test.go), built from the same samples.
// They pin each *_event.ts and input_command.ts type to its Go struct: a Go uint64 decodes as a
// bigint, every other number as a number.
const dir = new URL('../../../shards/game/test/testdata/wire/', import.meta.url)
const read = (name: string) => new Uint8Array(readFileSync(new URL(`${name}.msgpack`, dir)))

const bodySync: BodySync = {
  Tick: 9007199254740993n,
  TickSeconds: 0.05,
  SimVersion: 'go1.27.1+0123456789abcdef',
  MapID: 'open',
  You: {
    Body: 1,
    Team: 1,
    X: 1.5,
    Y: -2,
    Radius: 0.5,
    InputSession: 4294967295,
    InputAck: 42,
    InputQueued: 3,
    InputCap: 10,
    InputTarget: 4,
    InputHealth: { Coasts: 7, DroppedFrames: 2, OversizedInputs: 1, FlooredViews: 4294967295 },
    MoveSpeed: 5,
    SnapSeconds: 0.6,
    FadeSeconds: 0.125,
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
        Stacking: 1,
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
        Stacking: 0,
      },
    ],
    EffectsNext: 4294967295,
    Shots: [
      {
        Type: 'bolt',
        Ability: 'bolt',
        Seq: 4294967295,
        Launch: 1,
        PrevX: -4.75,
        PrevY: 2.125,
        X: -4.25,
        Y: 1.5,
        VelX: 9,
        VelY: -12,
        Radius: 0.15,
        Remaining: 17.75,
      },
    ],
    EchoDelay: 100,
    EchoSerial: 9007199254740993n,
    EchoPath: [
      { X: -3.5, Y: 0.25 },
      { X: -3.25, Y: 0.5 },
    ],
  },
  Bodies: [
    { EntityID: 1, Serial: 1n, PlayerID: 1, Nickname: 'Player One', Champion: 'starter', Effects: [] },
    {
      ...{ EntityID: 2, Serial: 9007199254740993n, PlayerID: 2, Nickname: 'Échø ✦', Champion: 'rook' },
      Effects: [
        { Kind: 'out', End: 18446744073709551615n },
        { Kind: 'stun', End: 13n },
      ],
    },
  ],
}
const echoSync: EchoSync = {
  Tick: 7n,
  Echoes: [
    { EntityID: 3, Serial: 3n, PlayerID: 1, Effects: [{ Kind: 'phase', End: 9007199254740993n }], HP: 62.5, MaxHP: 100 },
    { EntityID: 4, Serial: 4n, PlayerID: 2, Effects: [], HP: 0, MaxHP: 100 },
  ],
}
const abilitySync: AbilitySync = {
  Tick: 7n,
  Abilities: [{ Body: 1, Slot: 0, Ability: 'bolt', ReadyTick: 18446744073709551615n }],
}
const projectileSync: ProjectileSync = {
  Tick: 8n,
  Projectiles: [
    {
      ...{ EntityID: 9, Serial: 18446744073709551615n, Type: 'bolt', Caster: 4294967295, Team: 1 },
      ...{ PrevX: -0.5, PrevY: -6.75, X: 0.125, Y: -7.5, Radius: 0.15, Until: 1, Ended: false },
      ...{ OnTarget: false, TargetSerial: 0n, OffsetX: 0, OffsetY: 0 },
    },
    {
      ...{ EntityID: 10, Serial: 12n, Type: 'bolt', Caster: 3, Team: 0 },
      ...{ PrevX: 2.25, PrevY: 0.5, X: 2.5, Y: 1, Radius: 0.15, Until: 0.375, Ended: true },
      ...{ OnTarget: true, TargetSerial: 4n, OffsetX: -0.625, OffsetY: 0.125 },
    },
  ],
}
const projectileSyncEmpty: ProjectileSync = { Tick: 9n, Projectiles: [] }
const targetSync: TargetSync = {
  Tick: 9007199254740993n,
  Targets: [
    { EntityID: 0, Serial: 1n, Kind: BodyKind, Owner: 0, Team: 0, X: 1.5, Y: -2, Radius: 0.5, Tags: 3n, Until: 1 },
    { EntityID: 4, Serial: 18446744073709551615n, Kind: EchoKind, Owner: 2, Team: 1, X: 3, Y: -1.25, Radius: 0.5,
      Tags: 0n, Until: 0.625 },
  ],
}
const input: Input = {
  Session: 305419896,
  Frames: [
    { Seq: 1, MoveX: 1, MoveY: 0, AimX: 0, AimY: 0, Buttons: 0, ViewTick: 0 },
    {
      Seq: 2,
      MoveX: 0.7071067811865476,
      MoveY: -0.7071067811865476,
      AimX: 3.5,
      AimY: -1.25,
      Buttons: 1,
      ViewTick: 41.375,
    },
  ],
}

test.each([
  ['body_sync', bodySync],
  ['echo_sync', echoSync],
  ['ability_sync', abilitySync],
  ['projectile_sync', projectileSync],
  ['projectile_sync_empty', projectileSyncEmpty],
  ['target_sync', targetSync],
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
