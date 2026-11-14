import { useState } from 'react'
import type { FormEvent } from 'react'
import type { NewIntern } from '../../api/client'
import type { AddInternResponse, Person } from '../../api/schemas'
import { hireError, LOGIN_RE, loginFromName, useAddIntern } from './hire'

const directions: [NewIntern['direction'], string][] = [
  ['DIRECTION_BACKEND', 'Backend'],
  ['DIRECTION_FRONTEND', 'Frontend'],
  ['DIRECTION_QA', 'QA'],
]

const today = () => new Date().toISOString().slice(0, 10)

/** For managers: «Добавить стажёра», a short form, then the login and one-time password to hand over. */
export function HireIntern({ team }: { team: Person[] }) {
  const [open, setOpen] = useState(false)
  const [done, setDone] = useState<AddInternResponse | null>(null)

  if (done) return <Hired result={done} onClose={() => setDone(null)} />
  if (!open) {
    return (
      <button type="button" className="task-start" onClick={() => setOpen(true)}>
        Добавить стажёра
      </button>
    )
  }
  return (
    <HireForm
      mentors={team.filter((p) => !/стаж[её]р/i.test(p.role_title))}
      onCancel={() => setOpen(false)}
      onDone={(r) => {
        setOpen(false)
        setDone(r)
      }}
    />
  )
}

function HireForm({
  mentors,
  onCancel,
  onDone,
}: {
  mentors: Person[]
  onCancel: () => void
  onDone: (r: AddInternResponse) => void
}) {
  const [name, setName] = useState('')
  const [login, setLogin] = useState('')
  const [loginTouched, setLoginTouched] = useState(false)
  const [direction, setDirection] = useState<NewIntern['direction']>('DIRECTION_BACKEND')
  const [mentor, setMentor] = useState('')
  const [start, setStart] = useState(today)
  const [problem, setProblem] = useState('')
  const add = useAddIntern()
  const shownLogin = loginTouched ? login : loginFromName(name)

  const submit = (e: FormEvent) => {
    e.preventDefault()
    if (!name.trim() || !shownLogin || !mentor) {
      setProblem('Укажи имя, логин и наставника.')
      return
    }
    if (!LOGIN_RE.test(shownLogin)) {
      setProblem('Логин: латиница в нижнем регистре, цифры, «.», «_» или «-», от 3 до 32 символов, начинается с буквы.')
      return
    }
    setProblem('')
    add.mutate(
      { display_name: name.trim(), login: shownLogin, direction, mentor_id: mentor, start_date: start },
      { onSuccess: onDone },
    )
  }

  const error = problem || (add.isError ? hireError(add.error, shownLogin) : '')
  const pending = add.isPending

  return (
    <form className="task-form" aria-label="Новый стажёр" onSubmit={submit} noValidate>
      <label className="task-form__field">
        <span className="label">Имя и фамилия *</span>
        <input className="task-form__input" value={name} onChange={(e) => setName(e.target.value)} disabled={pending} />
      </label>
      <label className="task-form__field">
        <span className="label">Логин *</span>
        <input
          className="task-form__input"
          value={shownLogin}
          onChange={(e) => {
            setLoginTouched(true)
            setLogin(e.target.value)
          }}
          placeholder="vnovikova"
          autoCapitalize="off"
          spellCheck={false}
          disabled={pending}
        />
      </label>
      <label className="task-form__field">
        <span className="label">Направление *</span>
        <select
          className="task-form__input"
          value={direction}
          onChange={(e) => setDirection(e.target.value as NewIntern['direction'])}
          disabled={pending}
        >
          {directions.map(([value, text]) => (
            <option key={value} value={value}>
              {text}
            </option>
          ))}
        </select>
      </label>
      <label className="task-form__field">
        <span className="label">Наставник *</span>
        <select className="task-form__input" value={mentor} onChange={(e) => setMentor(e.target.value)} disabled={pending}>
          <option value="">Выбери наставника</option>
          {mentors.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
      </label>
      <label className="task-form__field">
        <span className="label">Дата выхода</span>
        <input
          className="task-form__input"
          type="date"
          value={start}
          onChange={(e) => setStart(e.target.value)}
          disabled={pending}
        />
      </label>
      {error && (
        <p className="qs qs--error" role="alert">
          {error}
        </p>
      )}
      <div className="plan-card__actions">
        <button type="submit" className="person__ask" disabled={pending}>
          {pending ? 'Добавляем…' : 'Добавить'}
        </button>
        <button type="button" className="plan-card__status" onClick={onCancel} disabled={pending}>
          Отмена
        </button>
      </div>
    </form>
  )
}

function Hired({ result, onClose }: { result: AddInternResponse; onClose: () => void }) {
  const [copied, setCopied] = useState(false)
  const copy = () => {
    void navigator.clipboard
      ?.writeText(`Логин: ${result.login}\nПароль: ${result.temporary_password}`)
      .then(() => setCopied(true))
      .catch(() => {})
  }
  return (
    <section className="task-form hired" aria-label="Стажёр добавлен">
      <p className="hired__title">{result.user.display_name} в команде</p>
      <dl className="hired__creds">
        <div>
          <dt className="label">Логин</dt>
          <dd>{result.login}</dd>
        </div>
        <div>
          <dt className="label">Одноразовый пароль</dt>
          <dd>{result.temporary_password}</dd>
        </div>
      </dl>
      <p className="info__note">
        Пароль показывается один раз и нигде не хранится: передай его стажёру лично. Он войдёт через обычный экран
        входа, план и buddy будут готовы сразу.
      </p>
      <div className="plan-card__actions">
        <button type="button" className="person__ask" onClick={copy}>
          {copied ? 'Скопировано' : 'Скопировать логин и пароль'}
        </button>
        <button type="button" className="plan-card__status" onClick={onClose}>
          Готово
        </button>
      </div>
    </section>
  )
}
