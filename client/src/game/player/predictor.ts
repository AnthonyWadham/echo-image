import type { Vec2 } from '../../engine/math/vec2'
import { runGo } from '../../engine/wasm/go'
import type { BodyState, BodySync } from './body_sync_event'
import type { FrameInput } from './input_stream'
import wasmUrl from './predictor.wasm?url'

/** One of your shots after the newest tick: where it is, and where it was a tick before. */
export interface PredictedShot {
  Type: string
  X: number
  Y: number
  PrevX: number
  PrevY: number
  Radius: number
}

/** Your kit after the newest tick, at that tick: when each slot's ability is ready, slot 0 first. */
export interface PredictedKit {
  Tick: number
  Slots: { Ability: string; ReadyTick: number }[]
}

/** The server's own code for your Body and what it casts: shards/game/predictor, built to wasm. */
export interface Predictor {
  /** Starts over from your Body as a body_sync left it. */
  reset(sync: BodySync, you: BodyState): void
  /** Where a frame's move would leave your Body next tick, without stepping: aim from there. */
  peek(moveX: number, moveY: number): Vec2
  /** Runs the next tick for a frame, as the server will, and returns your Body's new center. */
  step(frame: FrameInput): Vec2
  /** Your shots after the newest tick. */
  shots(): PredictedShot[]
  /** Your kit after the newest tick. */
  kit(): PredictedKit
}

/** What shards/game/predictor/main.go sets on globalThis. */
interface Exports {
  reset(state: object): void
  peek(moveX: number, moveY: number): Vec2
  step(frame: FrameInput): Vec2
  shots(): PredictedShot[]
  kit(): PredictedKit
}

/** Starts the predictor from wasm bytes, by default the build next to this file. */
export async function loadPredictor(wasm?: BufferSource): Promise<Predictor> {
  await runGo(wasm ?? (await (await fetch(wasmUrl)).arrayBuffer()))
  const globals = globalThis as { echoPredictor?: Exports; echoPredictorError?: string }
  const exports = globals.echoPredictor
  if (!exports) throw new Error(globals.echoPredictorError ?? 'predictor.wasm ran but set no echoPredictor')
  return {
    // A uint64 goes as a decimal string, since a number can't hold every one.
    reset: (sync, you) => {
      const { Body, MoveSpeed, Champion, Cooldowns, Effects, EffectsNext, Shots } = sync.You
      exports.reset({
        Tick: String(sync.Tick),
        Body,
        Team: you.Team,
        X: you.X,
        Y: you.Y,
        Radius: you.Radius,
        MoveSpeed,
        Champion,
        Cooldowns: Cooldowns.map((c) => ({ ...c, ReadyTick: String(c.ReadyTick) })),
        Effects: Effects.map((e) => ({
          ...e,
          Tags: String(e.Tags),
          Pass: String(e.Pass),
          Start: String(e.Start),
          End: String(e.End),
        })),
        EffectsNext,
        Shots,
      })
    },
    peek: (moveX, moveY) => exports.peek(moveX, moveY),
    step: (frame) => exports.step(frame),
    shots: () => exports.shots(),
    kit: () => exports.kit(),
  }
}
