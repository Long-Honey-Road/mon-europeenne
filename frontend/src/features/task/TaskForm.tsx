import { useState } from 'react'
import type { FormEvent } from 'react'
import { useMe } from '../auth/useMe'
import { useStartTask } from './useTasks'
import './task.css'

/** «Разобрать рабочую задачу»: key, optional title and repository, then the task thread. */
export function TaskForm() {
  const [open, setOpen] = useState(false)
  const repos = useMe().data?.user.repos ?? []
  const [key, setKey] = useState('')
  const [title, setTitle] = useState('')
  const [repo, setRepo] = useState<string | null>(null)
  const start = useStartTask()
  const repoValue = repo ?? repos[0] ?? ''

  if (!open) {
    return (
      <button type="button" className="task-start" onClick={() => setOpen(true)}>
        Разобрать рабочую задачу
      </button>
    )
  }

  const submit = (e: FormEvent) => {
    e.preventDefault()
    const k = key.trim().toUpperCase()
    if (!k) return
    start.mutate({
      key: k,
      ...(title.trim() ? { title: title.trim() } : {}),
      ...(repoValue.trim() ? { repo: repoValue.trim() } : {}),
    })
  }

  return (
    <form className="task-form" aria-label="Новая рабочая задача" onSubmit={submit}>
      <label className="task-form__field">
        <span className="label">Ключ задачи *</span>
        <input
          className="task-form__input"
          value={key}
          onChange={(e) => setKey(e.target.value)}
          placeholder="MSG-142"
          required
          disabled={start.isPending}
        />
      </label>
      <label className="task-form__field">
        <span className="label">Название</span>
        <input
          className="task-form__input"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Коротко, о чём задача"
          disabled={start.isPending}
        />
      </label>
      <label className="task-form__field">
        <span className="label">Репозиторий</span>
        <input
          className="task-form__input"
          value={repoValue}
          onChange={(e) => setRepo(e.target.value)}
          list="task-repos"
          disabled={start.isPending}
        />
        <datalist id="task-repos">
          {repos.map((r) => (
            <option key={r} value={r} />
          ))}
        </datalist>
      </label>
      {start.isError && (
        <p className="qs qs--error" role="alert">
          Не получилось начать разбор. Попробуй ещё раз.
        </p>
      )}
      <div className="plan-card__actions">
        <button type="submit" className="person__ask" disabled={!key.trim() || start.isPending}>
          {start.isPending ? 'Создаём…' : 'Начать разбор'}
        </button>
        <button type="button" className="plan-card__status" onClick={() => setOpen(false)}>
          Отмена
        </button>
      </div>
    </form>
  )
}
