import {
  Color,
  DirectionalLight,
  GridHelper,
  HemisphereLight,
  OrthographicCamera,
  Plane,
  Raycaster,
  Scene,
  Vector2,
  Vector3,
  WebGLRenderer,
} from 'three'
import type { Vec2 } from '../math/vec2'

/** A box, lowest corner first. */
export interface Bounds {
  min: Vec2
  max: Vec2
}

// Server (X, Y) is world (x, y). The ground is z = 0, and z points up out of it.
const ViewHeight = 20 // world units top to bottom (Unity orthographicSize 10)
const MaxPixelRatio = 2
const BoundsMargin = 1 // world units of ground shown past the bounds the camera keeps to
const ground = new Plane(new Vector3(0, 0, 1), 0)

/** The arena in three.js, seen top-down through an orthographic camera. */
export class Stage {
  readonly canvas: HTMLCanvasElement
  /** Holds the entity views. */
  readonly scene = new Scene()
  readonly #renderer = new WebGLRenderer({ antialias: true })
  // Unrotated, it looks down -z with +y up the screen, so the arena reads like a map.
  readonly #camera = new OrthographicCamera(-1, 1, 1, -1, 0.1, 100)
  readonly #raycaster = new Raycaster()
  #size = { width: 0, height: 0, pixelRatio: 0 }

  constructor(container: HTMLElement) {
    this.canvas = this.#renderer.domElement
    this.canvas.style.display = 'block'
    container.append(this.canvas)
    this.scene.background = new Color(0x101418)
    const grid = new GridHelper(100, 100, 0x3b4754, 0x232a32)
    grid.rotation.x = Math.PI / 2 // from the xz plane onto the ground
    const sky = new HemisphereLight(0xdfe8ff, 0x1e252c, 1.5)
    sky.position.set(0, 0, 1) // the sky is +z
    const sun = new DirectionalLight(0xffffff, 2)
    sun.position.set(-4, 6, 10)
    this.scene.add(grid, sky, sun)
    this.#camera.position.set(0, 0, 20)
    this.#camera.updateMatrixWorld()
    this.#fit()
  }

  /** The point on the ground under a pointer, in client pixels. The canvas fills the window. */
  groundPoint(pointer: Vec2): Vec2 | undefined {
    this.#fit()
    const { width, height } = this.#size
    const ndc = new Vector2((pointer.x / width) * 2 - 1, 1 - (pointer.y / height) * 2)
    this.#raycaster.setFromCamera(ndc, this.#camera)
    const hit = this.#raycaster.ray.intersectPlane(ground, new Vector3())
    return hit ? { x: hit.x, y: hit.y } : undefined
  }

  /**
   * Centres the view on at, but shows no more than BoundsMargin past bounds: it stops at their edge, and
   * centres on them along an axis they don't fill.
   */
  follow(at: Vec2, { min, max }: Bounds): void {
    this.#fit()
    const camera = this.#camera
    camera.position.x = centre(at.x, camera.right, min.x - BoundsMargin, max.x + BoundsMargin)
    camera.position.y = centre(at.y, camera.top, min.y - BoundsMargin, max.y + BoundsMargin)
    camera.updateMatrixWorld()
  }

  /** Draws the scene as the views left it. */
  render(): void {
    this.#fit()
    this.#renderer.render(this.scene, this.#camera)
  }

  // Sizes the canvas to the window and pixel ratio, keeping ViewHeight units top to bottom.
  #fit(): void {
    const width = Math.max(innerWidth, 1)
    const height = Math.max(innerHeight, 1)
    const pixelRatio = Math.min(devicePixelRatio, MaxPixelRatio)
    const size = this.#size
    if (width === size.width && height === size.height && pixelRatio === size.pixelRatio) return
    this.#size = { width, height, pixelRatio }
    this.#renderer.setPixelRatio(pixelRatio)
    this.#renderer.setSize(width, height)
    const halfWidth = (ViewHeight / 2) * (width / height)
    const camera = this.#camera
    camera.left = -halfWidth
    camera.right = halfWidth
    camera.top = ViewHeight / 2
    camera.bottom = -ViewHeight / 2
    camera.updateProjectionMatrix()
  }
}

/** Where to centre a view half wide along an axis, near at, so it shows nothing outside lo to hi. */
export function centre(at: number, half: number, lo: number, hi: number): number {
  if (hi - lo <= 2 * half) return (lo + hi) / 2
  return Math.min(Math.max(at, lo + half), hi - half)
}
