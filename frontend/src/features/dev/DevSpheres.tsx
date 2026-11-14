import { useCallback, useEffect, useRef, useState } from 'react'
import { openThreadEvents } from '../../api/events'
import { useSession } from '../auth/session'
import { devCall } from './devApi'
import type { DevMessage, DevPin, DevVaultItem, Focus } from './devApi'

type Tab = 'plan' | 'vault'
type PlanEntry = { item_id: string; title: string; status: string }

const threadType = { plan: 'THREAD_TYPE_PLAN', vault: 'THREAD_TYPE_VAULT' } as const
const focusKind = { plan: 'FOCUS_KIND_PLAN_ITEM', vault: 'FOCUS_KIND_VAULT_ITEM' } as const

/** Service page to check spheres by hand: focus, sphere memory, pins, highlights and the vault. No design. */
export function DevSpheres() {
  const token = useSession((s) => s.token)
  const [tab, setTab] = useState<Tab>('plan')
  const [threadId, setThreadId] = useState<string | null>(null)
  const [focus, setFocus] = useState<Focus>(null)
  const [plan, setPlan] = useState<PlanEntry[]>([])
  const [vault, setVault] = useState<DevVaultItem[]>([])
  const [record, setRecord] = useState<DevVaultItem | null>(null)
  const [pins, setPins] = useState<DevPin[]>([])
  const [messages, setMessages] = useState<DevMessage[]>([])
  const [events, setEvents] = useState<string[]>([])
  const [text, setText] = useState('')
  const [error, setError] = useState('')
  const busy = useRef(false)

  const log = (line: string) => setEvents((e) => [line, ...e].slice(0, 30))
  const fail = (e: unknown) => setError(e instanceof Error ? e.message : String(e))

  const loadMessages = useCallback(async (id: string) => {
    const r = await devCall<{ messages: DevMessage[] }>('GET', `/v1/private/threads/${id}/messages`)
    setMessages(r.messages)
  }, [])

  const loadSide = useCallback(async () => {
    const [p, v] = await Promise.all([
      devCall<{ items: PlanEntry[] }>('GET', '/v1/private/plan'),
      devCall<{ items: DevVaultItem[] }>('GET', '/v1/private/vault'),
    ])
    setPlan(p.items)
    setVault(v.items)
  }, [])

  useEffect(() => {
    if (!token) return
    setFocus(null)
    setRecord(null)
    devCall<{ thread: { id: string } }>('POST', '/v1/private/threads', { type: threadType[tab] })
      .then(async (r) => {
        setThreadId(r.thread.id)
        await Promise.all([loadMessages(r.thread.id), loadSide()])
      })
      .catch(fail)
  }, [tab, token, loadMessages, loadSide])

  useEffect(() => {
    if (!threadId || !token) return
    const ctrl = new AbortController()
    const stream = openThreadEvents({
      baseUrl: import.meta.env.VITE_API_URL ?? '',
      token,
      threadId,
      signal: ctrl.signal,
      onEvent: (e) => {
        if (e.type === 'delta') return
        log(`${e.type} ${JSON.stringify(e.data)}`)
        if (e.type === 'done' || e.type === 'error') {
          busy.current = false
          void loadMessages(threadId)
          void loadSide()
        }
      },
    })
    // The page only logs events; a dropped stream is not an error here.
    stream.closed.catch(() => {})
    return () => ctrl.abort()
  }, [threadId, token, loadMessages, loadSide])

  useEffect(() => {
    if (!focus) {
      setPins([])
      setRecord(null)
      return
    }
    if (focus.kind === 'FOCUS_KIND_PLAN_ITEM') {
      devCall<{ pins: DevPin[] }>('GET', `/v1/private/pins?kind=${focus.kind}&id=${encodeURIComponent(focus.id)}`)
        .then((r) => setPins(r.pins))
        .catch(fail)
    } else {
      devCall<{ item: DevVaultItem }>('GET', `/v1/private/vault/${focus.id}`)
        .then((r) => setRecord(r.item))
        .catch(fail)
    }
  }, [focus, events.length])

  const send = async () => {
    if (!threadId || !text.trim() || busy.current) return
    busy.current = true
    setError('')
    try {
      await devCall('POST', `/v1/private/threads/${threadId}/messages`, {
        content: text,
        mode: 'ANSWER_MODE_BRIEF',
        ...(focus ? { focus } : {}),
      })
      setText('')
      await loadMessages(threadId)
    } catch (e) {
      busy.current = false
      fail(e)
    }
  }

  const act = (method: string, path: string, body?: unknown) =>
    devCall(method, path, body)
      .then(() => {
        log(`${method} ${path} ok`)
        void loadSide()
        if (focus) setFocus({ ...focus })
      })
      .catch(fail)

  const shown = messages.filter((m) => !focus || (m.focus?.id === focus.id))

  return (
    <main style={{ display: 'grid', gridTemplateColumns: '280px 1fr 320px', gap: 16, padding: 16, fontSize: 14 }}>
      <section aria-label="Сферы">
        <div role="tablist">
          {(['plan', 'vault'] as const).map((t) => (
            <button key={t} role="tab" aria-selected={tab === t} onClick={() => setTab(t)}>
              {t === 'plan' ? 'План' : 'Хранилище'}
            </button>
          ))}
        </div>
        <ul>
          <li>
            <button onClick={() => setFocus(null)} aria-pressed={focus === null}>
              Обзор
            </button>
          </li>
          {tab === 'plan'
            ? plan.map((p) => (
                <li key={p.item_id}>
                  <button
                    aria-pressed={focus?.id === p.item_id}
                    onClick={() => setFocus({ kind: focusKind.plan, id: p.item_id })}
                  >
                    {p.item_id} · {p.title} · {p.status}
                  </button>
                </li>
              ))
            : vault.map((v) => (
                <li key={v.id}>
                  <button aria-pressed={focus?.id === v.id} onClick={() => setFocus({ kind: focusKind.vault, id: v.id })}>
                    {v.title} · v{v.version}
                  </button>
                </li>
              ))}
        </ul>
      </section>

      <section aria-label="Чат вкладки">
        <p>
          Фокус: <strong>{focus ? focus.id : 'обзор'}</strong>
        </p>
        <ol>
          {shown.map((m) => (
            <li key={m.id}>
              <strong>{m.role === 'MESSAGE_ROLE_USER' ? 'Ты' : 'buddy'}</strong>
              {m.focus ? ` [${m.focus.id}]` : ''}: {m.text}
              {m.role === 'MESSAGE_ROLE_ASSISTANT' && (
                <>
                  {' '}
                  {tab === 'plan' && (
                    <button
                      onClick={() =>
                        act('POST', `/v1/private/messages/${m.id}/pin`, focus?.kind === focusKind.plan ? { focus } : {})
                      }
                    >
                      Закрепить
                    </button>
                  )}
                  <button onClick={() => act('POST', `/v1/private/messages/${m.id}/vault`, {})}>В хранилище</button>
                </>
              )}
            </li>
          ))}
        </ol>
        <textarea aria-label="Сообщение" value={text} onChange={(e) => setText(e.target.value)} rows={3} cols={60} />
        <button onClick={() => void send()}>Отправить</button>
        {error && <p role="alert">{error}</p>}
      </section>

      <aside aria-label="Детали">
        {focus?.kind === 'FOCUS_KIND_PLAN_ITEM' && (
          <>
            <h3>Закрепы</h3>
            <ul aria-label="Закрепы">
              {pins.map((p) => (
                <li key={p.id}>
                  <strong>{p.title}</strong>: {p.text}{' '}
                  <button onClick={() => act('DELETE', `/v1/private/pins/${p.id}`)}>удалить</button>
                </li>
              ))}
            </ul>
          </>
        )}
        {record && (
          <>
            <h3>
              {record.title} · v{record.version}
            </h3>
            <pre style={{ whiteSpace: 'pre-wrap' }}>{record.summary}</pre>
            <ul aria-label="Версии">
              {(record.versions ?? []).map((v) => (
                <li key={v.version}>
                  v{v.version}{' '}
                  <button onClick={() => act('POST', `/v1/private/vault/${record.id}/restore`, { version: v.version })}>
                    вернуть
                  </button>
                </li>
              ))}
            </ul>
          </>
        )}
        <h3>События</h3>
        <ol aria-label="События" style={{ fontFamily: 'monospace', fontSize: 12 }}>
          {events.map((e, i) => (
            <li key={i}>{e}</li>
          ))}
        </ol>
      </aside>
    </main>
  )
}
