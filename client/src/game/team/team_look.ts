import { White } from '../../engine/render/color'
import type { Rgb } from '../../engine/render/color'

const TeamColors: readonly Rgb[] = [
  [0.2, 0.5, 1],
  [1, 0.3, 0.25],
]

const teamColor = (team: number): Rgb => TeamColors[team % TeamColors.length] ?? White

/** One look per team, made from its colour on first use, so a team's views share it. */
export class TeamLooks<T> {
  readonly #make: (color: Rgb) => T
  readonly #looks = new Map<number, T>()

  constructor(make: (color: Rgb) => T) {
    this.#make = make
  }

  get(team: number): T {
    const found = this.#looks.get(team)
    if (found !== undefined) return found
    const look = this.#make(teamColor(team))
    this.#looks.set(team, look)
    return look
  }
}
