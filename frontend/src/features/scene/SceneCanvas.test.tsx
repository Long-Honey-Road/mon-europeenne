import { afterEach, describe, expect, it, vi } from 'vitest'
import { render } from '@testing-library/react'
import { SceneCanvas } from './SceneCanvas'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

function fakeContext() {
  return {
    setTransform: vi.fn(),
    clearRect: vi.fn(),
    fillRect: vi.fn(),
    beginPath: vi.fn(),
    arc: vi.fn(),
    fill: vi.fn(),
  }
}

describe('SceneCanvas', () => {
  it('draws a frame around the orbs it finds and stops on unmount', () => {
    const ctx = fakeContext()
    vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(ctx as unknown as CanvasRenderingContext2D)
    const frames: FrameRequestCallback[] = []
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => frames.push(cb))
    const cancel = vi.fn()
    vi.stubGlobal('cancelAnimationFrame', cancel)

    const { unmount } = render(
      <div>
        <span data-orb-ball data-state="main" data-color="#00aaff" />
        <SceneCanvas />
      </div>,
    )
    expect(ctx.setTransform).toHaveBeenCalled()
    expect(frames).toHaveLength(1)

    frames[0](1000)
    expect(ctx.clearRect).toHaveBeenCalled()
    expect(ctx.fillRect).toHaveBeenCalled()
    expect(frames).toHaveLength(2)

    unmount()
    expect(cancel).toHaveBeenCalled()
  })

  it('does nothing without a 2d context', () => {
    const raf = vi.fn()
    vi.stubGlobal('requestAnimationFrame', raf)
    render(<SceneCanvas />)
    expect(raf).not.toHaveBeenCalled()
  })
})
