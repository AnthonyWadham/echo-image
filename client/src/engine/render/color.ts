import { Color, MeshStandardMaterial, SRGBColorSpace } from 'three'

/** An sRGB colour, as in Unity. */
export type Rgb = readonly [number, number, number]

export const White: Rgb = [1, 1, 1]

export const mix = (a: Rgb, b: Rgb, t: number): Rgb => [
  a[0] + (b[0] - a[0]) * t,
  a[1] + (b[1] - a[1]) * t,
  a[2] + (b[2] - a[2]) * t,
]

/** A lit material in this colour. */
export const lit = ([r, g, b]: Rgb) =>
  new MeshStandardMaterial({ color: new Color().setRGB(r, g, b, SRGBColorSpace) })
