import { Link } from '@tanstack/react-router'
import type { RequestStatus } from '../../api/schemas'
import { QueryState } from '../../shared/ui/QueryState'
import { useMyRequests } from '../requests/useRequests'
import { useTaskProgress } from './progress'
import { taskSteps } from './steps'
import { TaskForm } from './TaskForm'
import { taskOf, useThreads } from './useTasks'

const requestStatus: Record<RequestStatus, { tone: string; text: string }> = {
  REQUEST_STATUS_UNSPECIFIED: { tone: 'todo', text: 'Отправлено' },
  REQUEST_STATUS_PENDING: { tone: 'waiting', text: 'Ждёт согласования' },
  REQUEST_STATUS_APPROVED: { tone: 'done', text: 'Одобрено' },
  REQUEST_STATUS_REJECTED: { tone: 'waiting', text: 'Отклонено' },
}

/** Live content of the «Задачи» orb: work task threads and the user's requests. */
export function WorkPanel() {
  const threads = useThreads()
  const requests = useMyRequests()
  const progress = useTaskProgress((s) => s.done)

  return (
    <>
      <p className="label info__subhead">Рабочие задачи</p>
      <QueryState
        query={threads}
        isEmpty={(list) => !list.some((t) => taskOf(t))}
        empty="Рабочих задач пока нет."
      >
        {(list) => (
          <ul className="info__list" aria-label="Рабочие задачи">
            {list.flatMap((t) => {
              const task = taskOf(t)
              if (!task) return []
              const passed = progress[t.id]?.length ?? 0
              return [
                <li key={t.id}>
                  <Link
                    to="/task/$threadId"
                    params={{ threadId: t.id }}
                    className="info__item info__item--button"
                  >
                    <span className="info__text">
                      <span className="info__item-title">
                        {task.key}
                        {task.title ? ` · ${task.title}` : ''}
                      </span>
                      {task.repo && <span className="info__note">{task.repo}</span>}
                    </span>
                    <span className={`info__status info__status--${passed === taskSteps.length ? 'done' : 'active'}`}>
                      {passed} из {taskSteps.length}
                    </span>
                  </Link>
                </li>,
              ]
            })}
          </ul>
        )}
      </QueryState>
      <TaskForm />

      <p className="label info__subhead">Заявки</p>
      <QueryState
        query={requests}
        isEmpty={(list) => list.length === 0}
        empty="Заявок пока нет: их готовит buddy, когда нужен доступ."
      >
        {(list) => (
          <ul className="info__list" aria-label="Заявки">
            {list.map((r) => {
              const st = requestStatus[r.status]
              return (
                <li key={r.id} className="info__item">
                  <span className="info__text">
                    <span className="info__item-title">{r.title}</span>
                    <span className="info__note">
                      {r.id}
                      {r.reason ? ` · ${r.reason}` : r.next_step ? ` · ${r.next_step}` : ''}
                    </span>
                  </span>
                  <span className={`info__status info__status--${st.tone}`}>{st.text}</span>
                </li>
              )
            })}
          </ul>
        )}
      </QueryState>
    </>
  )
}
