import { useState } from 'react'
import type { PlanItem, PlanItemStatus, PlanPeriod } from '../../api/schemas'
import { QueryState } from '../../shared/ui/QueryState'
import { useChatRoute } from '../chat/route'
import { itemStatus, periodTitle } from '../today/useToday'
import { usePlan, useSetItemStatus } from './usePlan'

const periodOrder: PlanPeriod[] = [
  'PLAN_PERIOD_W1',
  'PLAN_PERIOD_W2',
  'PLAN_PERIOD_M1',
  'PLAN_PERIOD_M2',
  'PLAN_PERIOD_EQUATOR',
  'PLAN_PERIOD_FINAL',
  'PLAN_PERIOD_UNSPECIFIED',
]

/** Live content of the «План» orb: items by stage, an item card with status changes, a link to the plan thread. */
export function PlanPanel({ onAsk }: { onAsk: (text: string) => void }) {
  const plan = usePlan()
  const [open, setOpen] = useState<string | null>(null)

  return (
    <QueryState
      query={plan}
      isEmpty={(p) => p.items.length === 0}
      empty="План пока пуст: buddy соберёт его вместе с наставником."
    >
      {(p) => {
        const titles = new Map(p.items.map((i) => [i.id, i.title]))
        return (
          <>
            <p className="info__meta">
              Сделано {p.done} из {p.total}
            </p>
            {periodOrder.map((period) => {
              const items = p.items.filter((i) => i.period === period)
              if (items.length === 0) return null
              const title = periodTitle[period] || 'Без этапа'
              return (
                <div key={period} className="plan-stage">
                  <p className="label info__subhead">{title}</p>
                  <ul className="info__list" aria-label={title}>
                    {items.map((item) => (
                      <PlanRow
                        key={item.id}
                        item={item}
                        blockedBy={item.blocked_by.map((id) => titles.get(id) ?? id)}
                        expanded={open === item.id}
                        onToggle={() => setOpen(open === item.id ? null : item.id)}
                        onAsk={onAsk}
                      />
                    ))}
                  </ul>
                </div>
              )
            })}
          </>
        )
      }}
    </QueryState>
  )
}

const nextStatuses = (s: PlanItemStatus): { status: PlanItemStatus; text: string }[] => {
  if (s === 'PLAN_ITEM_STATUS_DONE') return [{ status: 'PLAN_ITEM_STATUS_DOING', text: 'Вернуть в работу' }]
  if (s === 'PLAN_ITEM_STATUS_DOING') return [{ status: 'PLAN_ITEM_STATUS_DONE', text: 'Отметить сделанным' }]
  return [
    { status: 'PLAN_ITEM_STATUS_DOING', text: 'Начать' },
    { status: 'PLAN_ITEM_STATUS_DONE', text: 'Отметить сделанным' },
  ]
}

function PlanRow({
  item,
  blockedBy,
  expanded,
  onToggle,
  onAsk,
}: {
  item: PlanItem
  blockedBy: string[]
  expanded: boolean
  onToggle: () => void
  onAsk: (text: string) => void
}) {
  const status = itemStatus[item.status]
  const setStatus = useSetItemStatus()
  const openThread = useChatRoute((s) => s.open)
  const cardId = `plan-item-${item.id}`

  return (
    <li>
      <button
        type="button"
        className="info__item info__item--button"
        aria-expanded={expanded}
        aria-controls={cardId}
        onClick={onToggle}
      >
        <span className="info__text">
          <span className="info__item-title">{item.title}</span>
        </span>
        <span className={`info__status info__status--${status.tone}`}>{status.text}</span>
      </button>
      {expanded && (
        <div id={cardId} role="group" aria-label={item.title} className="person__details plan-card">
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
      )}
    </li>
  )
}
