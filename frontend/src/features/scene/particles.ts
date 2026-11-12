export type Particle = {
  orb: number
  angle: number
  speed: number
  /** Orbit radius in orb radii. */
  radius: number
  tilt: number
  /** Vertical squash of the orbit: makes it read as a ring seen from the side. */
  squash: number
  size: number
  alpha: number
}

export type Star = { x: number; y: number; size: number; phase: number; speed: number }

export type OrbCircle = { cx: number; cy: number; r: number; color: string; boost: number }

/** Small seeded PRNG so the sky looks the same on every load. */
export function seededRandom(seed: number) {
  let a = seed
  return () => {
    a = (a + 0x6d2b79f5) | 0
    let t = Math.imul(a ^ (a >>> 15), 1 | a)
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

export function createStars(count: number, rand: () => number): Star[] {
  return Array.from({ length: count }, () => ({
    x: rand(),
    y: rand(),
    size: 0.6 + rand() * 1.4,
    phase: rand() * Math.PI * 2,
    speed: 0.3 + rand() * 0.9,
  }))
}

export function createParticles(orbs: number, perOrb: number, rand: () => number): Particle[] {
  return Array.from({ length: orbs * perOrb }, (_, i) => ({
    orb: i % orbs,
    angle: rand() * Math.PI * 2,
    speed: (0.12 + rand() * 0.35) * (rand() < 0.5 ? -1 : 1),
    radius: 1.12 + rand() * 0.75,
    tilt: -0.5 + rand() * 1,
    squash: 0.22 + rand() * 0.3,
    size: 0.7 + rand() * 1.6,
    alpha: 0.35 + rand() * 0.65,
  }))
}

/** Position of a particle at time `t` (seconds) and whether it is on the near side of the orb. */
export function particlePoint(p: Particle, orb: OrbCircle, t: number) {
  const a = p.angle + t * p.speed * orb.boost
  const ex = Math.cos(a) * p.radius * orb.r
  const ey = Math.sin(a) * p.radius * orb.r * p.squash
  const cos = Math.cos(p.tilt)
  const sin = Math.sin(p.tilt)
  return { x: orb.cx + ex * cos - ey * sin, y: orb.cy + ex * sin + ey * cos, front: Math.sin(a) > 0 }
}

export function drawScene(
  ctx: CanvasRenderingContext2D,
  width: number,
  height: number,
  stars: Star[],
  particles: Particle[],
  orbs: OrbCircle[],
  t: number,
) {
  ctx.clearRect(0, 0, width, height)

  ctx.globalCompositeOperation = 'source-over'
  ctx.fillStyle = '#ffffff'
  for (const s of stars) {
    ctx.globalAlpha = 0.18 + 0.5 * (0.5 + 0.5 * Math.sin(t * s.speed + s.phase))
    ctx.fillRect(s.x * width, s.y * height, s.size, s.size)
  }

  ctx.globalCompositeOperation = 'lighter'
  for (const p of particles) {
    const orb = orbs[p.orb]
    if (!orb) continue
    const { x, y, front } = particlePoint(p, orb, t)
    // The far half of the orbit is hidden behind the ball.
    if (!front && Math.hypot(x - orb.cx, y - orb.cy) < orb.r) continue
    const size = p.size * (front ? 1 : 0.7) * Math.max(0.6, Math.min(1.6, orb.r / 90))
    ctx.fillStyle = orb.color
    ctx.globalAlpha = p.alpha * (front ? 0.22 : 0.1)
    ctx.beginPath()
    ctx.arc(x, y, size * 3.2, 0, Math.PI * 2)
    ctx.fill()
    ctx.globalAlpha = p.alpha * (front ? 1 : 0.45)
    ctx.beginPath()
    ctx.arc(x, y, size, 0, Math.PI * 2)
    ctx.fill()
  }
  ctx.globalAlpha = 1
}
