import type { Vec2 } from '../../engine/math/vec2'
import { runGo } from '../../engine/wasm/go'
import type { TargetSync } from '../projectile/target_sync_event'
import type { BodyState, BodySync } from './body_sync_event'
import type { InputFrame } from './input_command'
import wasmUrl from './predictor.wasm?url'

/** One of your shots after the newest tick: its key, where it is, and where it was a tick before. */
export interface PredictedShot {
  Type: string
  Seq: number
  Ability: string
  Launch: number
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

/** Your Echo after the newest tick, and where it was a tick before. */
export interface PredictedEcho {
  X: number
  Y: number
  PrevX: number
  PrevY: number
}

/**
 * One of your shots that ended in the newest tick: X, Y is where it ended, Until (0 to 1) through the
 * tick. One that met a Body or Echo (OnTarget) did so Offset from the center of the one with serial
 * TargetSerial.
 */
export interface EndedShot extends PredictedShot {
  Until: number
  OnTarget: boolean
  TargetSerial: bigint
  OffsetX: number
  OffsetY: number
}

/** A frame's tick: where it leaves your Body, and your shots that ended in it. */
export interface Stepped {
  at: Vec2
  ended: EndedShot[]
}

/** The server's own code for your Body and what it casts: shards/game/predictor, built to wasm. */
export interface Predictor {
  /** The sim it runs: the SimVersion a shard built from the same tree sends. */
  readonly version: string
  /** How far behind the newest tick to draw others: the content's, which the server's rewind allows. */
  readonly interpolationSeconds: number
  /** Starts over from your Body as a body_sync left it. */
  reset(sync: BodySync, you: BodyState): void
  /** Keeps the targets a target_sync shows, so your shots meet them where your screen drew them. */
  seeTargets(sync: TargetSync): void
  /** Forgets the targets it saw: after a reconnect or a server restart. */
  forget(): void
  /** Where a frame's move would leave your Body next tick, without stepping: aim from there. */
  peek(moveX: number, moveY: number): Vec2
  /** Runs the next tick for a frame, new or replayed, as the server will. Shots meet others at its ViewTick. */
  step(frame: InputFrame): Stepped
  /** Your shots after the newest tick. */
  shots(): PredictedShot[]
  /** Your Echo after the newest tick, or undefined with none. */
  echo(): PredictedEcho | undefined
  /** Your kit after the newest tick. */
  kit(): PredictedKit
}

/** What shards/game/predictor/main.go sets on globalThis. */
interface Exports {
  version: string
  reset(state: object): void
  seeTargets(sync: object): void
  forget(): void
  peek(moveX: number, moveY: number): Vec2
  step(frame: InputFrame): Vec2 & { ended: (Omit<EndedShot, 'TargetSerial'> & { TargetSerial: string })[] }
  shots(): PredictedShot[]
  echo(): PredictedEcho | null
  kit(): PredictedKit
  useContent(files: Record<string, string>): string | null
  interpolationSeconds(): number
}

/**
 * Starts the predictor from wasm bytes, by default the build next to this file. It plays the content
 * built into it, with content's files (by path, as JSON) in place of its own: a test's.
 */
export async function loadPredictor(wasm?: BufferSource, content?: Record<string, string>): Promise<Predictor> {
  await runGo(wasm ?? (await (await fetch(wasmUrl)).arrayBuffer()))
  const globals = globalThis as { echoPredictor?: Exports; echoPredictorError?: string }
  const exports = globals.echoPredictor
  if (!exports) throw new Error(globals.echoPredictorError ?? 'predictor.wasm ran but set no echoPredictor')
  const refused = content && exports.useContent(content)
  if (refused) throw new Error(refused)
  const interpolationSeconds = exports.interpolationSeconds()
  return {
    version: exports.version,
    interpolationSeconds,
    // A uint64 goes as a decimal string, since a number can't hold every one.
    reset: (sync, you) => {
      const { Body, MoveSpeed, Champion, Cooldowns, Effects, EffectsNext, Shots, EchoDelay, EchoPath } = sync.You
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
        EchoDelay,
        EchoPath,
      })
    },
    seeTargets: (sync) =>
      exports.seeTargets({
        Tick: String(sync.Tick),
        Targets: sync.Targets.map((t) => ({ ...t, Serial: String(t.Serial), Kind: String(t.Kind), Tags: String(t.Tags) })),
      }),
    forget: () => exports.forget(),
    peek: (moveX, moveY) => exports.peek(moveX, moveY),
    step: (frame) => {
      const { x, y, ended } = exports.step(frame)
      return { at: { x, y }, ended: ended.map((e) => ({ ...e, TargetSerial: BigInt(e.TargetSerial) })) }
    },
    shots: () => exports.shots(),
    echo: () => exports.echo() ?? undefined,
    kit: () => exports.kit(),
  }
}
