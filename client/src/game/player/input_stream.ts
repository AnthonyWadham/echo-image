import type { Vec2 } from '../../engine/math/vec2'
import type { YouState } from './body_sync_event'
import type { Input, InputFrame } from './input_command'

const MaxStepSeconds = 0.15 // after a hitch, don't fire a burst of inputs
const UnackedSeconds = 6.4 // of frames: past this the server isn't answering
const ResendSeconds = 0.4 // of unacked frames repeated in each Input, covering late or lost ones
const QueueTargetSeconds = 0.05 // waiting on the server after each tick: margin for jitter
const QueueSeconds = 0.5 // how long the server's queue depth is averaged over
const DilationPerSecond = 1 // how much faster or slower the frame clock runs per second queued off target
const MaxDilation = 0.1

// The frames in seconds, at tickSeconds a frame.
const framesIn = (seconds: number, tickSeconds: number) => Math.round(seconds / tickSeconds)

/** A frame's input: everything but its Seq. */
export type FrameInput = Omit<InputFrame, 'Seq'>

/** A random nonzero uint32, new per page load, so the server can tell a reloaded client. */
export function newSession(): number {
  const [session = 0] = crypto.getRandomValues(new Uint32Array(1))
  return session === 0 ? newSession() : session
}

/**
 * This run's input: Seq numbers, the frames the server hasn't applied, and when to send. Its frame clock
 * is paced by the server's queue (Overwatch's time dilation): a little fast while the queue runs dry,
 * a little slow while it fills, so each frame lands about QueueTargetSeconds before the server applies it.
 */
export class InputStream {
  readonly session: number
  #seq = 0
  #unacked: InputFrame[] = []
  #sentMove: Vec2 = { x: 0, y: 0 }
  #stepTime = 0
  #queued = QueueTargetSeconds // the server's queue depth in seconds, averaged
  #cap = 1 // the most frames an Input may carry: the server's queue (YouState.InputCap)

  constructor(session: number) {
    this.session = session
  }

  /** Frames sent but not yet applied by the server, oldest first. */
  get unacked(): readonly InputFrame[] {
    return this.#unacked
  }

  /** Seconds of dt saved toward the next frame. */
  get stepTime(): number {
    return this.#stepTime
  }

  /** How fast the frame clock runs: above 1 while the server's queue is short of the target. */
  get rate(): number {
    const dilation = DilationPerSecond * (QueueTargetSeconds - this.#queued)
    return 1 + Math.min(Math.max(dilation, -MaxDilation), MaxDilation)
  }

  /** A move-only frame. Sent on connect, it claims a free Body. */
  join(move: Vec2): Input {
    this.#sentMove = move
    const frame = this.#frame({ MoveX: move.x, MoveY: move.y, AimX: 0, AimY: 0, Buttons: 0, ViewTick: 0 })
    return { Session: this.session, Frames: [frame] }
  }

  /** Before playing: a move-only frame when the move direction changes. */
  moveChanged(move: Vec2): Input | undefined {
    const same = move.x === this.#sentMove.x && move.y === this.#sentMove.y
    return same ? undefined : this.join(move)
  }

  /**
   * While playing: a frame of next(seq) per tickSeconds of dt, each sent with the newest unacked frames, no
   * more than the server queues.
   */
  step(dt: number, tickSeconds: number, next: (seq: number) => FrameInput): Input[] {
    if (!(tickSeconds > 0)) return []
    const frames = (seconds: number) => framesIn(seconds, tickSeconds)
    this.#stepTime = Math.min(this.#stepTime + dt * this.rate, frames(MaxStepSeconds) * tickSeconds)
    const sends: Input[] = []
    while (this.#stepTime >= tickSeconds) {
      this.#stepTime -= tickSeconds
      const seq = ++this.#seq
      this.#unacked.push({ Seq: seq, ...next(seq) })
      if (this.#unacked.length > frames(UnackedSeconds)) this.#unacked.shift()
      sends.push({ Session: this.session, Frames: this.#unacked.slice(-Math.min(frames(ResendSeconds), this.#cap)) })
    }
    return sends
  }

  /**
   * Keeps how many frames the server queues, drops the frames it applied, and averages its queue depth
   * over the tick. Another session's ack (our last run) says nothing of those.
   */
  ack(you: YouState, tickSeconds: number): void {
    this.#cap = you.InputCap
    if (you.InputSession !== this.session) return
    this.#unacked = this.#unacked.filter((f) => f.Seq > you.InputAck)
    const weight = 1 - Math.exp(-tickSeconds / QueueSeconds)
    if (weight > 0) this.#queued += (you.InputQueued * tickSeconds - this.#queued) * weight
  }

  /** Forgets unacked frames, step time and pacing. Seq carries on, so the server never sees one twice. */
  reset(): void {
    this.#unacked = []
    this.#stepTime = 0
    this.#queued = QueueTargetSeconds
  }

  #frame(input: FrameInput): InputFrame {
    return { Seq: ++this.#seq, ...input }
  }
}
