import { useEffect, useRef } from 'react'
import { createParticles, createStars, drawScene, seededRandom } from './particles'
import type { OrbCircle } from './particles'

const STARS = 150
const PARTICLES_PER_ORB = 30
const MAX_DPR = 2

/** Stars and orbiting particles. One canvas above the orbs, redrawn every frame. */
export function SceneCanvas() {
  const ref = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const canvas = ref.current
    const ctx = canvas?.getContext('2d')
    const scene = canvas?.parentElement
    if (!canvas || !ctx || !scene) return

    const balls = Array.from(scene.querySelectorAll<HTMLElement>('[data-orb-ball]'))
    const rand = seededRandom(7)
    const stars = createStars(STARS, rand)
    const particles = createParticles(balls.length, PARTICLES_PER_ORB, rand)
    const boosts = balls.map(() => 1)
    let width = 0
    let height = 0

    const resize = () => {
      const dpr = Math.min(window.devicePixelRatio || 1, MAX_DPR)
      width = canvas.clientWidth
      height = canvas.clientHeight
      canvas.width = Math.round(width * dpr)
      canvas.height = Math.round(height * dpr)
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    }

    const readOrbs = (): OrbCircle[] => {
      const origin = canvas.getBoundingClientRect()
      return balls.map((ball, i) => {
        const box = ball.getBoundingClientRect()
        const lively = ball.dataset.state === 'main' || ball.matches(':hover') ? 2.4 : 1
        boosts[i] += (lively - boosts[i]) * 0.06
        return {
          cx: box.left - origin.left + box.width / 2,
          cy: box.top - origin.top + box.height / 2,
          r: box.width / 2,
          color: ball.dataset.color ?? '#ffffff',
          boost: boosts[i],
        }
      })
    }

    resize()
    const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(resize)
    observer?.observe(canvas)

    const still = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
    let frame = 0
    const tick = (ms: number) => {
      drawScene(ctx, width, height, stars, particles, readOrbs(), ms / 1000)
      if (!still) frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)

    return () => {
      cancelAnimationFrame(frame)
      observer?.disconnect()
    }
  }, [])

  return <canvas ref={ref} className="scene__canvas" aria-hidden="true" />
}
