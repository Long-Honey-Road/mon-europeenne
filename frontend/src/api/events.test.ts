import { describe, expect, it } from 'vitest'
import { openThreadEvents, parseFrames } from './events'
import type { ThreadEvent } from './events'

describe('parseFrames', () => {
  it('reads named events, skips comments and keeps an unfinished frame', () => {
    const got: ThreadEvent[] = []
    const rest = parseFrames(
      ': connected\n\nevent: delta\ndata: {"text":"При"}\n\n: ping\n\ndata: {"a":1}\r\n\r\nevent: done\ndata: {"te',
      (e) => got.push(e),
    )
    expect(got).toEqual([
      { type: 'delta', data: { text: 'При' } },
      { type: 'message', data: { a: 1 } },
    ])
    expect(rest).toBe('event: done\ndata: {"te')
  })

  it('joins multi-line data and skips what is not a JSON object', () => {
    const got: ThreadEvent[] = []
    parseFrames('event: block\ndata: {"block":\ndata: {"type":"x"}}\n\nevent: bad\ndata: {oops\n\ndata: 42\n\nevent: x\n\n', (e) => got.push(e))
    expect(got).toEqual([{ type: 'block', data: { block: { type: 'x' } } }])
  })
})

describe('openThreadEvents', () => {
  it('fails when the stream ends before it connected', async () => {
    const body = new ReadableStream<Uint8Array>({
      start(c) {
        c.enqueue(new TextEncoder().encode('event: x\ndata: {}\n\n'))
        c.close()
      },
    })
    const stream = openThreadEvents({
      baseUrl: '',
      token: 't',
      threadId: 'th',
      signal: new AbortController().signal,
      onEvent: () => {},
      fetch: async () => new Response(body),
    })
    await expect(stream.ready).rejects.toThrow('closed before it connected')
    await expect(stream.closed).rejects.toThrow()
  })
})
