import type { PlanItem, PlanItemStatus } from '../../api/schemas'
import { useChatRoute } from '../chat/route'
import { useSetItemStatus } from './usePlan'

const nextStatuses = (s: PlanItemStatus): { status: PlanItemStatus; text: string }[] => {
  if (s === 'PLAN_ITEM_STATUS_DONE') return [{ status: 'PLAN_ITEM_STATUS_DOING', text: 'Вернуть в работу' }]
  if (s === 'PLAN_ITEM_STATUS_DOING') return [{ status: 'PLAN_ITEM_STATUS_DONE', text: 'Отметить сделанным' }]
  return [
    { status: 'PLAN_ITEM_STATUS_DOING', text: 'Начать' },
    { status: 'PLAN_ITEM_STATUS_DONE', text: 'Отметить сделанным' },
  ]
}

/** One plan item: practice, note, what it waits for, status buttons and a question to the plan thread. */
export function PlanItemCard({
  item,
  blockedBy,
  onAsk,
  id,
  className = '',
  children,
}: {
  item: PlanItem
  blockedBy: string[]
  onAsk: (text: string) => void
  id?: string
  className?: string
  children?: React.ReactNode
}) {
  const setStatus = useSetItemStatus()
  const openThread = useChatRoute((s) => s.open)
  return (
    <div id={id} role="group" aria-label={item.title} className={`plan-card ${className}`}>
      {children}
      {item.practice && <p>{item.practice}</p>}
      {item.note && (
        <p>
          <span className="label">Заметка</span> {item.note}
        </p>
      )}
      {blockedBy.length > 0 && <p className="info__note">Ждёт: {blockedBy.join(', ')}</p>}
      {setStatus.isError && (
        <p className="qs qs--error" role="alert">
          Статус не сохранился. Попробуй ещё раз.
        </p>
      )}
      <div className="plan-card__actions">
        {nextStatuses(item.status).map((n) => (
          <button
            key={n.status}
            type="button"
            className="plan-card__status"
            disabled={setStatus.isPending}
            onClick={() => setStatus.mutate({ id: item.id, status: n.status })}
          >
            {n.text}
          </button>
        ))}
        <button
          type="button"
          className="person__ask"
          onClick={() => {
            openThread('THREAD_TYPE_PLAN')
            onAsk(`Расскажи про пункт плана «${item.title}»: что сделать?`)
          }}
        >
          Обсудить в треде плана
        </button>
      </div>
    </div>
  )
}
