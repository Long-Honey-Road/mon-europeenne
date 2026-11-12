import { describe, expect, it, vi } from 'vitest'
import { createParticles, createStars, drawScene, particlePoint, seededRandom } from './particles'
import type { OrbCircle, Particle } from './particles'

const orb: OrbCircle = { cx: 100, cy: 100, r: 50, color: '#04e061', boost: 1 }
const particle: Particle = { orb: 0, angle: 0, speed: 1, radius: 2, tilt: 0, squash: 0.5, size: 1, alpha: 1 }

function fakeContext() {
  return {
    clearRect: vi.fn(),
    fillRect: vi.fn(),
    beginPath: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
    globalAlpha: 1,
    globalCompositeOperation: 'source-over',
    fillStyle: '',
  }
}

describe('seededRandom', () => {
  it('is deterministic and stays in [0, 1)', () => {
    const a = seededRandom(7)
    const b = seededRandom(7)
    for (let i = 0; i < 50; i++) {
      const value = a()
      expect(value).toBe(b())
      expect(value).toBeGreaterThanOrEqual(0)
      expect(value).toBeLessThan(1)
    }
  })
})

describe('generators', () => {
  it('spreads particles evenly across orbs', () => {
    const particles = createParticles(4, 10, seededRandom(1))
    expect(particles).toHaveLength(40)
    for (let i = 0; i < 4; i++) expect(particles.filter((p) => p.orb === i)).toHaveLength(10)
    expect(particles.every((p) => p.radius > 1)).toBe(true)
  })

  it('creates stars in normalised coordinates', () => {
    const stars = createStars(20, seededRandom(1))
    expect(stars).toHaveLength(20)
    expect(stars.every((s) => s.x >= 0 && s.x < 1 && s.y >= 0 && s.y < 1)).toBe(true)
  })
})

describe('particlePoint', () => {
  it('starts on the orbit radius to the right of the orb', () => {
    const point = particlePoint(particle, orb, 0)
    expect(point.x).toBeCloseTo(200)
    expect(point.y).toBeCloseTo(100)
  })

  it('squashes the orbit vertically and reports the near side', () => {
    const point = particlePoint(particle, orb, Math.PI / 2)
    expect(point.x).toBeCloseTo(100)
    expect(point.y).toBeCloseTo(150)
    expect(point.front).toBe(true)
    expect(particlePoint(particle, orb, -Math.PI / 2).front).toBe(false)
  })

  it('moves faster when the orb is boosted', () => {
    const boosted = particlePoint(particle, { ...orb, boost: 2 }, Math.PI / 4)
    expect(boosted.x).toBeCloseTo(100)
  })
})

describe('drawScene', () => {
  it('draws every star and a halo plus a core per visible particle', () => {
    const ctx = fakeContext()
    drawScene(ctx as unknown as CanvasRenderingContext2D, 300, 200, createStars(5, seededRandom(1)), [particle], [orb], 0)
    expect(ctx.clearRect).toHaveBeenCalledWith(0, 0, 300, 200)
    expect(ctx.fillRect).toHaveBeenCalledTimes(5)
    expect(ctx.arc).toHaveBeenCalledTimes(2)
    expect(ctx.globalAlpha).toBe(1)
  })

  it('hides far-side particles behind the ball and skips orphans', () => {
    const ctx = fakeContext()
    const behind: Particle = { ...particle, radius: 0.5, angle: -Math.PI / 2 }
    const orphan: Particle = { ...particle, orb: 3 }
    drawScene(ctx as unknown as CanvasRenderingContext2D, 300, 200, [], [behind, orphan], [orb], 0)
    expect(ctx.arc).not.toHaveBeenCalled()
  })
})
