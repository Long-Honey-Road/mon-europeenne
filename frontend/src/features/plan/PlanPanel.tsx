import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import type { PlanItem } from '../../api/schemas'
import { QueryState } from '../../shared/ui/QueryState'
import { itemStatus, periodTitle } from '../today/useToday'
import { periodOrder } from './layout'
import { PlanItemCard } from './PlanItemCard'
import { usePlan } from './usePlan'

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
            <p className="info__meta plan-meta">
              <span>
                Сделано {p.done} из {p.total}
              </span>
              <Link to="/plan/map" className="plan-meta__map">
                Открыть карту
              </Link>
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
        <PlanItemCard id={cardId} item={item} blockedBy={blockedBy} onAsk={onAsk} className="person__details" />
      )}
    </li>
  )
}
