import type { Vec2 } from '../../engine/math/vec2'
import type { Spot } from '../../engine/render/entity_views'

/** How a shot ended: on a Body or Echo (OnTarget), the one with serial TargetSerial, Offset from its center. */
export interface TargetEnd {
  OnTarget: boolean
  TargetSerial: bigint
  OffsetX: number
  OffsetY: number
}

/**
 * Where a shot that ended on a Body or Echo ends on this screen: on it, where targetAt says this screen
 * draws it now. Undefined if it ended on none, or this screen doesn't draw it.
 */
export function endOn(e: TargetEnd, targetAt: (serial: bigint) => Vec2 | undefined): Spot | undefined {
  const at = e.OnTarget ? targetAt(e.TargetSerial) : undefined
  return at && { X: at.x + e.OffsetX, Y: at.y + e.OffsetY }
}
