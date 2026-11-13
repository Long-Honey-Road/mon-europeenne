import { useEffect } from 'react'
import { Link } from '@tanstack/react-router'
import { QueryState } from '../../shared/ui/QueryState'
import { UserMenu } from '../auth/UserMenu'
import { useChatRoute } from '../chat/route'
import { useTaskProgress } from './progress'
import { taskSteps } from './steps'
import { taskOf, useThreads } from './useTasks'
import './task.css'

const EMPTY: number[] = []

/** /task/:threadId: the work task, its four steps and the task thread in the chat. */
export function TaskScreen({ threadId, onAsk }: { threadId: string; onAsk: (text: string) => void }) {
  const threads = useThreads()
  const openId = useChatRoute((s) => s.openId)
  const passed = useTaskProgress((s) => s.done[threadId] ?? EMPTY)
  const toggle = useTaskProgress((s) => s.toggle)

  const thread = threads.data?.find((t) => t.id === threadId)
  const task = thread && taskOf(thread)

  // Once per task screen: refetched threads give a new object, not a new task.
  const taskKey = task?.key
  useEffect(() => {
    if (taskKey) openId(threadId)
  }, [taskKey, threadId, openId])

  return (
    <section className="task-screen" aria-label={task ? `Задача ${task.key}` : 'Задача'}>
      <header className="plan-map__top">
        <Link to="/" className="scene__back">
          <span aria-hidden="true">←</span> Все разделы
        </Link>
        <div className="scene__actions">
          <Link to="/work" className="scene__back">
            Задачи
          </Link>
          <UserMenu />
        </div>
      </header>
      <div className="task-screen__body">
        <QueryState query={threads} empty={null}>
          {() =>
            !task ? (
              <p className="qs qs--empty">Задача не найдена: возможно, тред удалён или открыт другим пользователем.</p>
            ) : (
              <>
                <p className="label">Рабочая задача{task.repo ? ` · ${task.repo}` : ''}</p>
                <h1 className="task-screen__title">
                  <span className="task-screen__key">{task.key}</span> {task.title}
                </h1>
                <p className="info__lead">
                  buddy ведёт по шагам и показывает, где смотреть в репозитории и документах команды. Код пишешь ты.
                </p>
                <p className="info__meta">
                  Пройдено {passed.length} из {taskSteps.length}
                </p>
                <ol className="task-steps" aria-label="Шаги разбора">
                  {taskSteps.map((step, i) => {
                    const done = passed.includes(i)
                    return (
                      <li key={step.title} className="task-step" data-done={done}>
                        <span className="task-step__num" aria-hidden="true">
                          {done ? '✓' : i + 1}
                        </span>
                        <div className="task-step__body">
                          <h3 className="task-step__title">{step.title}</h3>
                          <p className="info__note">{step.hint}</p>
                          <div className="plan-card__actions">
                            <button type="button" className="person__ask" onClick={() => onAsk(step.prompt(task))}>
                              Спросить buddy
                            </button>
                            <button type="button" className="plan-card__status" onClick={() => toggle(threadId, i)}>
                              {done ? 'Вернуть' : 'Отметить пройденным'}
                            </button>
                          </div>
                        </div>
                      </li>
                    )
                  })}
                </ol>
              </>
            )
          }
        </QueryState>
      </div>
    </section>
  )
}
