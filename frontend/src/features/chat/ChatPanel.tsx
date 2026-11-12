import { useState } from 'react'
import type { FormEvent, RefObject } from 'react'
import { useCurrentUser } from '../auth/useMe'
import type { AppSection } from '../nav/sections'
import './chat.css'

type Message = { id: number; from: 'buddy' | 'user'; text: string }

const welcome = (name?: string): Message => ({
  id: 0,
  from: 'buddy',
  text: `Привет${name ? `, ${name}` : ''}! Спроси про задачи, доступы или процессы команды.`,
})
// The model is not wired yet: every question gets this honest placeholder.
const STUB_REPLY = 'Модель ещё не подключена: это каркас интерфейса. Здесь будет ответ с источниками.'

export function ChatPanel({
  context,
  draft,
  onDraftChange,
  inputRef,
}: {
  /** Section in focus on the scene, shown as the chat context. */
  context?: AppSection
  draft: string
  onDraftChange: (text: string) => void
  inputRef: RefObject<HTMLInputElement>
}) {
  const me = useCurrentUser()
  const [messages, setMessages] = useState<Message[]>([])
  const shown = [welcome(me?.firstName), ...messages]

  const send = (e: FormEvent) => {
    e.preventDefault()
    const text = draft.trim()
    if (!text) return
    setMessages((prev) => [
      ...prev,
      { id: prev.length + 1, from: 'user', text },
      { id: prev.length + 2, from: 'buddy', text: STUB_REPLY },
    ])
    onDraftChange('')
  }

  return (
    <aside className="chat" aria-label="Чат с buddy">
      <header className="chat__head">
        <div>
          <h2 className="chat__title">buddy</h2>
          <p className="chat__sub">ИИ-наставник · учебные данные</p>
        </div>
        {me && (
          <div className="chat__user">
            <span className="chat__avatar" aria-hidden="true">
              {me.name[0]}
            </span>
            <span className="chat__who">
              {me.name}
              <span className="chat__role">{me.subtitle}</span>
            </span>
          </div>
        )}
      </header>

      <ol className="chat__log" aria-live="polite">
        {shown.map((m) => (
          <li key={m.id} className={`msg msg--${m.from}`}>
            {m.text}
          </li>
        ))}
      </ol>

      <form className="chat__form" onSubmit={send}>
        {context && (
          <p className="chat__context" data-accent={context.accent}>
            Контекст: {context.title}
          </p>
        )}
        <div className="chat__row">
          <label className="visually-hidden" htmlFor="chat-input">
            Сообщение для buddy
          </label>
          <input
            id="chat-input"
            ref={inputRef}
            className="chat__input"
            placeholder="Спроси buddy…"
            value={draft}
            onChange={(e) => onDraftChange(e.target.value)}
            autoComplete="off"
          />
          <button type="submit" className="chat__send" aria-label="Отправить" disabled={!draft.trim()}>
            <span aria-hidden="true">↑</span>
          </button>
        </div>
      </form>
    </aside>
  )
}
