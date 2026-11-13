import { useEffect, useRef, useState } from 'react'
import type { FormEvent, RefObject } from 'react'
import Markdown from 'react-markdown'
import type { AnswerMode } from '../../api/client'
import type { ChatMessage, Thread, ThreadType } from '../../api/schemas'
import { useCurrentUser } from '../auth/useMe'
import type { AppSection } from '../nav/sections'
import { Blocks, Escalation } from './Blocks'
import { useChat } from './useChat'
import type { LiveTurn } from './useChat'
import './chat.css'

const threadTypeNames: Record<ThreadType, string> = {
  THREAD_TYPE_UNSPECIFIED: 'Тред',
  THREAD_TYPE_GENERAL: 'Общий',
  THREAD_TYPE_PLAN: 'План',
  THREAD_TYPE_REQUEST: 'Заявки',
  THREAD_TYPE_TASK: 'Задача',
}

// Thread types a user can start from the chat; task threads start from a task (S-08).
const newThreadTypes: ThreadType[] = ['THREAD_TYPE_GENERAL', 'THREAD_TYPE_PLAN', 'THREAD_TYPE_REQUEST']

const when = new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })

export function threadLabel(t: Thread): string {
  const created = new Date(t.created_at)
  const date = Number.isNaN(created.getTime()) ? '' : ` · ${when.format(created)}`
  return `${t.title || threadTypeNames[t.type]}${date}`
}

function Answer({ text }: { text: string }) {
  return (
    <div className="msg__text">
      <Markdown>{text}</Markdown>
    </div>
  )
}

function AssistantMessage({
  message,
  rating,
  onRate,
  onMore,
}: {
  message: ChatMessage
  rating?: 'up' | 'down'
  onRate: (value: 'up' | 'down') => void
  onMore?: () => void
}) {
  return (
    <li className="msg msg--buddy">
      <Answer text={message.text} />
      <Blocks blocks={message.blocks} />
      <div className="msg__actions">
        {onMore && (
          <button type="button" className="msg__more" onClick={onMore}>
            Подробнее
          </button>
        )}
        <button
          type="button"
          className="msg__rate"
          aria-label="Полезно"
          aria-pressed={rating === 'up'}
          onClick={() => onRate('up')}
        >
          👍
        </button>
        <button
          type="button"
          className="msg__rate"
          aria-label="Не помогло"
          aria-pressed={rating === 'down'}
          onClick={() => onRate('down')}
        >
          👎
        </button>
      </div>
    </li>
  )
}

function Live({ turn }: { turn: LiveTurn }) {
  return (
    <>
      <li className="msg msg--user">{turn.question}</li>
      <li className="msg msg--buddy" aria-busy={!turn.error}>
        {turn.text ? <Answer text={turn.text} /> : !turn.error && <span className="msg__typing" aria-hidden="true" />}
        {turn.status && !turn.error && <p className="msg__status">buddy {turn.status}…</p>}
        <Blocks blocks={turn.blocks} />
        {turn.error && (
          <div className="block block--warn" role="alert">
            <p className="block__title">{turn.error.message}</p>
            <Escalation steps={turn.error.escalation} />
          </div>
        )}
      </li>
    </>
  )
}

function ThreadPicker({
  threads,
  threadId,
  onSelect,
  onCreate,
  disabled,
}: {
  threads: Thread[]
  threadId: string | null
  onSelect: (id: string) => void
  onCreate: (type: ThreadType) => void
  disabled: boolean
}) {
  const [open, setOpen] = useState(false)
  return (
    <div className="threads">
      <label className="visually-hidden" htmlFor="chat-thread">
        Тред
      </label>
      <select
        id="chat-thread"
        className="threads__select"
        value={threadId ?? ''}
        onChange={(e) => onSelect(e.target.value)}
        disabled={disabled || threads.length === 0}
      >
        {threads.map((t) => (
          <option key={t.id} value={t.id}>
            {threadLabel(t)}
          </option>
        ))}
      </select>
      <button
        type="button"
        className="threads__new"
        aria-expanded={open}
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
      >
        Новый
      </button>
      {open && (
        <ul className="threads__menu" aria-label="Новый тред">
          {newThreadTypes.map((type) => (
            <li key={type}>
              <button
                type="button"
                onClick={() => {
                  setOpen(false)
                  onCreate(type)
                }}
              >
                {threadTypeNames[type]}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

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
  const chat = useChat()
  const [mode, setMode] = useState<AnswerMode>('brief')
  const log = useRef<HTMLOListElement>(null)

  const liveText = chat.live?.text
  const liveBlocks = chat.live?.blocks.length
  useEffect(() => {
    const el = log.current
    if (el) el.scrollTop = el.scrollHeight
  }, [chat.messages.length, liveText, liveBlocks, chat.live?.error])

  const send = (e: FormEvent) => {
    e.preventDefault()
    const text = draft.trim()
    if (!text || chat.busy) return
    onDraftChange('')
    void chat.send(text, mode)
  }

  // "Подробнее" asks the previous question again in the detailed mode.
  const questionBefore = (index: number) => {
    for (let i = index - 1; i >= 0; i--) {
      const m = chat.messages[i]
      if (m?.role === 'MESSAGE_ROLE_USER') return m.text
    }
    return undefined
  }

  const empty = chat.messages.length === 0 && !chat.live

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
      <ThreadPicker
        threads={chat.threads}
        threadId={chat.threadId}
        onSelect={chat.selectThread}
        onCreate={(type) => void chat.createThread(type).catch(() => {})}
        disabled={chat.busy}
      />

      <ol className="chat__log" aria-live="polite" ref={log}>
        {empty && (
          <li className="msg msg--buddy">
            {chat.loadError
              ? 'Не получилось загрузить переписку. Обнови страницу чуть позже.'
              : `Привет${me?.firstName ? `, ${me.firstName}` : ''}! Спроси про задачи, доступы или процессы команды.`}
          </li>
        )}
        {chat.messages.map((m, i) => {
          if (m.role !== 'MESSAGE_ROLE_ASSISTANT') {
            return (
              <li key={m.id} className="msg msg--user">
                {m.text}
              </li>
            )
          }
          const question = questionBefore(i)
          return (
            <AssistantMessage
              key={m.id}
              message={m}
              rating={chat.ratings[m.id]}
              onRate={(v) => void chat.rate(m.id, v)}
              onMore={question && !chat.busy ? () => void chat.send(question, 'detailed') : undefined}
            />
          )
        })}
        {chat.live && <Live turn={chat.live} />}
      </ol>

      <form className="chat__form" onSubmit={send}>
        <div className="chat__bar">
          {context && (
            <p className="chat__context" data-accent={context.accent}>
              Контекст: {context.title}
            </p>
          )}
          <div className="chat__mode" role="group" aria-label="Режим ответа">
            {(['brief', 'detailed'] as const).map((m) => (
              <button key={m} type="button" aria-pressed={mode === m} onClick={() => setMode(m)}>
                {m === 'brief' ? 'Кратко' : 'Подробно'}
              </button>
            ))}
          </div>
        </div>
        <div className="chat__row">
          <label className="visually-hidden" htmlFor="chat-input">
            Сообщение для buddy
          </label>
          <input
            id="chat-input"
            ref={inputRef}
            className="chat__input"
            placeholder={chat.busy ? 'buddy отвечает…' : 'Спроси buddy…'}
            value={draft}
            onChange={(e) => onDraftChange(e.target.value)}
            autoComplete="off"
          />
          <button
            type="submit"
            className="chat__send"
            aria-label="Отправить"
            disabled={!draft.trim() || chat.busy || !chat.threadId}
          >
            <span aria-hidden="true">↑</span>
          </button>
        </div>
      </form>
    </aside>
  )
}
