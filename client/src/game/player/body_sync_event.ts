import type { Message } from '../../engine/net/shard_connection'

// Mirrors the Go types field for field. ../wire_fixtures.test.ts fails when they drift.

export const BodySyncEvent: Message<BodySync> = { name: 'body_sync' }

/** player.BodySync: shards/game/internal/player/body_sync_event.go */
export interface BodySync {
  Tick: bigint // uint64
  TickSeconds: number
  SimVersion: string // the sim the shard runs: your predictor predicts only with the same
  You: YouState
  Bodies: BodyState[]
}

/** player.YouState: everything your predictor starts from. */
export interface YouState {
  Body: number // uint32 EntityID
  Team: number // uint8: your Body's row, which the predictor starts from: its team,
  X: number // spot
  Y: number
  Radius: number // and collider radius
  InputSession: number // uint32
  InputAck: number // uint32
  InputQueued: number // uint32: frames waiting on the server for later ticks, which pace yours
  InputCap: number // uint32: the most frames the server queues: it drops an Input with more
  InputTarget: number // uint32: frames to keep queued; it rises each time your queue runs dry
  InputHealth: InputHealth // what went wrong with your input stream
  MoveSpeed: number
  SnapSeconds: number // a correction longer than this much of MoveSpeed jumps
  FadeSeconds: number // a shorter one shrinks to 1/e in this long
  Champion: string // the champion your Body plays: its kit
  Cooldowns: CooldownEntry[] // when each ability your Body has cast is ready again
  Effects: EffectEntry[] // your Body's timed effects, for the predictor
  EffectsNext: number // uint32: the last serial your Body's effect list gave out
  Shots: ShotState[] // the shots in flight your predictor flies: your frames cast them
  EchoDelay: number // uint32: ticks your Echo trails your Body; 0 with no Echo
  EchoSerial: bigint // uint64: your Echo's serial, its row's in target_sync; 0 with no Echo
  EchoPath: { X: number; Y: number }[] // your Echo this tick, then its spot on each next tick
}

/** component.InputHealth: shards/game/internal/player/component/input.go. Each a uint32 count. */
export interface InputHealth {
  Coasts: number // times your queue ran dry while you played: each a correction
  DroppedFrames: number // frames dropped past the queue's cap
  OversizedInputs: number // Inputs dropped whole, for more frames than the queue holds
  FlooredViews: number // frames whose view went back, taken at the newest so far
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
  Stacking: number // status.Stacking: what its refresh key is
}

/** player.ShotState: one of your shots in flight. Ability, Seq and Launch are its key. */
export interface ShotState {
  Type: string // shot type ID: its rules
  Ability: string // the ability that cast it
  Seq: number // uint32: the input frame that cast it
  Launch: number // uint32: its place among that cast's launches
  PrevX: number // where its flight this tick began
  PrevY: number
  X: number
  Y: number
  VelX: number // units per second
  VelY: number
  Radius: number
  Remaining: number // distance left before it expires
}

/** player.BodyState: what a Body's row in target_sync lacks. Its row has its team, spot, size and tags. */
export interface BodyState {
  EntityID: number // uint32: names it in this tick's syncs
  Serial: bigint // uint64: names it across ticks, as its row's
  PlayerID: number // uint32
  Nickname: string
  Champion: string // what it looks like: visuals key by champion ID
  Effects: ShownEffect[] // its effect kinds from the next tick on, for their visuals
}

/** status.Shown: an effect kind as other screens see it. */
export interface ShownEffect {
  Kind: string // visuals key by effect kind
  End: bigint // uint64 tick: active until then
}
