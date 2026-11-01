import type { Message } from '../../engine/net/shard_connection'

// Mirrors the Go types field for field. ../wire_fixtures.test.ts fails when they drift.

export const InputCommand: Message<Input> = { name: 'input' }

/** player.Input: shards/game/internal/player/input_command.go */
export interface Input {
  Session: number // uint32
  Frames: InputFrame[]
}

/** component.InputFrame: shards/game/internal/player/component/input.go */
export interface InputFrame {
  Seq: number // uint32
  MoveX: number
  MoveY: number
  AimX: number
  AimY: number
  Buttons: number // uint32; bit N casts kit slot N
  ViewTick: number // the render tick on screen when the frame was made
  Claims: HitClaim[] // the shots this frame's step ended on Bodies and Echoes
}

/** component.HitClaim: a shot of yours this frame's step ended. Seq, Ability and Launch are its key. */
export interface HitClaim {
  Seq: number // uint32
  Ability: string
  Launch: number // uint32
  Target: number // uint32 EntityID: the Body or Echo it stopped on
}
