// Thread events over SSE. EventSource cannot send the Authorization header, so the stream is read with fetch.

export type ThreadEvent = { type: string; data: Record<string, unknown> }

export type StreamOptions = {
  baseUrl: string
  token: string
  threadId: string
  signal: AbortSignal
  onEvent: (e: ThreadEvent) => void
  fetch?: typeof fetch
}

/** Parses complete SSE frames from the buffer and returns what is left for the next chunk. */
export function parseFrames(buffer: string, onEvent: (e: ThreadEvent) => void): string {
  const frames = buffer.split(/\r?\n\r?\n/)
  const rest = frames.pop() ?? ''
  for (const frame of frames) {
    let type = 'message'
    const data: string[] = []
    for (const line of frame.split(/\r?\n/)) {
      if (line.startsWith(':')) continue
      const colon = line.indexOf(':')
      const field = colon < 0 ? line : line.slice(0, colon)
      const value = colon < 0 ? '' : line.slice(colon + 1).replace(/^ /, '')
      if (field === 'event') type = value
      if (field === 'data') data.push(value)
    }
    if (data.length === 0) continue
    try {
      const parsed: unknown = JSON.parse(data.join('\n'))
      if (parsed && typeof parsed === 'object') onEvent({ type, data: parsed as Record<string, unknown> })
    } catch {
      // A malformed frame is skipped; the turn still ends with done or error.
    }
  }
  return rest
}

/**
 * Opens the thread's event stream. `ready` resolves once the server is subscribed, so a message sent after it
 * cannot miss its events; `closed` resolves when the stream ends or is aborted.
 */
export function openThreadEvents(opts: StreamOptions): { ready: Promise<void>; closed: Promise<void> } {
  let markReady: () => void = () => {}
  let failReady: (e: unknown) => void = () => {}
  const ready = new Promise<void>((resolve, reject) => {
    markReady = resolve
    failReady = reject
  })
  // Callers may never await ready; keep the rejection handled.
  ready.catch(() => {})

  const run = async () => {
    const fetchImpl = opts.fetch ?? globalThis.fetch
    const res = await fetchImpl(`${opts.baseUrl}/v1/private/threads/${encodeURIComponent(opts.threadId)}/events`, {
      headers: { Authorization: `Bearer ${opts.token}`, Accept: 'text/event-stream' },
      signal: opts.signal,
    })
    if (!res.ok || !res.body) throw new Error(`event stream: HTTP ${res.status}`)
    // The server writes ": connected" only after it subscribed.
    const reader = res.body.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    let connected = false
    for (;;) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      if (!connected && /(^|\n): ?connected/.test(buffer)) {
        connected = true
        markReady()
      }
      buffer = parseFrames(buffer, opts.onEvent)
    }
    if (!connected) throw new Error('event stream closed before it connected')
  }

  const closed = run().then(
    () => undefined,
    (e: unknown) => {
      failReady(e)
      if (!opts.signal.aborted) throw e
    },
  )
  return { ready, closed }
}
