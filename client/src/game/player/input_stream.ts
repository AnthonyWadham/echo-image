import type { Vec2 } from '../../engine/math/vec2'
import type { YouState } from './body_sync_event'
import type { Input, InputFrame } from './input_command'

const MaxStepsPerFrame = 3 // after a hitch, don't fire a burst of inputs
const MaxUnacked = 128 // ~6 s of frames; past this the server isn't answering
const MaxResend = 8 // unacked frames repeated in each Input, covering late or lost ones

/** A frame's input: everything but its Seq. */
export type FrameInput = Omit<InputFrame, 'Seq'>

/** A random nonzero uint32, new per page load, so the server can tell a reloaded client. */
export function newSession(): number {
  const [session = 0] = crypto.getRandomValues(new Uint32Array(1))
  return session === 0 ? newSession() : session
}

/** This run's input: Seq numbers, the frames the server hasn't applied, and when to send. */
export class InputStream {
  readonly session: number
  #seq = 0
  #unacked: InputFrame[] = []
  #sentMove: Vec2 = { x: 0, y: 0 }
  #stepTime = 0

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

  /** A move-only frame. Sent on connect, it claims a free Body. */
  join(move: Vec2): Input {
    this.#sentMove = move
    const frame = this.#frame({ MoveX: move.x, MoveY: move.y, AimX: 0, AimY: 0, Buttons: 0, ViewTick: 0, Claims: [] })
    return { Session: this.session, Frames: [frame] }
  }

  /** Before playing: a move-only frame when the move direction changes. */
  moveChanged(move: Vec2): Input | undefined {
    const same = move.x === this.#sentMove.x && move.y === this.#sentMove.y
    return same ? undefined : this.join(move)
  }

  /** While playing: a frame of next(seq) per tickSeconds of dt, each sent with the newest unacked frames. */
  step(dt: number, tickSeconds: number, next: (seq: number) => FrameInput): Input[] {
    if (!(tickSeconds > 0)) return []
    this.#stepTime = Math.min(this.#stepTime + dt, MaxStepsPerFrame * tickSeconds)
    const sends: Input[] = []
    while (this.#stepTime >= tickSeconds) {
      this.#stepTime -= tickSeconds
      const seq = ++this.#seq
      this.#unacked.push({ Seq: seq, ...next(seq) })
      if (this.#unacked.length > MaxUnacked) this.#unacked.shift()
      sends.push({ Session: this.session, Frames: this.#unacked.slice(-MaxResend) })
    }
    return sends
  }

  /** Drops the frames the server applied. Another session's ack (our last run) says nothing. */
  ack(you: YouState): void {
    const ack = you.InputSession === this.session ? you.InputAck : 0
    this.#unacked = this.#unacked.filter((f) => f.Seq > ack)
  }

  /** Forgets unacked frames and step time. Seq carries on, so the server never sees one twice. */
  reset(): void {
    this.#unacked = []
    this.#stepTime = 0
  }

  #frame(input: FrameInput): InputFrame {
    return { Seq: ++this.#seq, ...input }
  }
}
