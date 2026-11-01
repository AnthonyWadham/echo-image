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
  ViewTick: number // the render tick on screen when the frame was made: your shots meet others there
}
