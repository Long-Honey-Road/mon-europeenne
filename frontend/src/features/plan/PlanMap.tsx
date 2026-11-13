import { useEffect, useMemo, useState } from 'react'
import type { CSSProperties } from 'react'
import { Link } from '@tanstack/react-router'
import type { Plan } from '../../api/schemas'
import { QueryState } from '../../shared/ui/QueryState'
import { UserMenu } from '../auth/UserMenu'
import { useChatRoute } from '../chat/route'
import { itemStatus } from '../today/useToday'
import { usePlanHighlight } from './highlight'
import { layoutPlan } from './layout'
import { PlanItemCard } from './PlanItemCard'
import { usePlan } from './usePlan'
import './map.css'

/** The plan tab: items as glowing spheres in stage constellations, dependencies as links, chat in the plan thread. */
export function PlanMap({ onAsk }: { onAsk: (text: string) => void }) {
  const plan = usePlan()
  const openThread = useChatRoute((s) => s.open)

  useEffect(() => {
    openThread('THREAD_TYPE_PLAN')
  }, [openThread])

  return (
    <section className="plan-map" aria-label="Карта плана">
      <header className="plan-map__top">
        <Link to="/" className="scene__back">
          <span aria-hidden="true">←</span> Все разделы
        </Link>
        <h1 className="plan-map__title">Карта плана</h1>
        <div className="scene__actions">
          <Link to="/plan" className="scene__back">
            Списком
          </Link>
          <UserMenu />
        </div>
      </header>
      <div className="plan-map__body">
        <QueryState
          query={plan}
          isEmpty={(p) => p.items.length === 0}
          empty="План пока пуст: buddy соберёт его вместе с наставником."
        >
          {(p) => <Constellations plan={p} onAsk={onAsk} />}
        </QueryState>
      </div>
    </section>
  )
}

const NARROW = '(max-width: 640px)'

/** True on phone widths, where a stage wraps into rows of three. */
function useNarrow() {
  const [narrow, setNarrow] = useState(() => window.matchMedia?.(NARROW).matches ?? false)
  useEffect(() => {
    const mq = window.matchMedia?.(NARROW)
    if (!mq) return
    const on = () => setNarrow(mq.matches)
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  }, [])
  return narrow
}

function Constellations({ plan, onAsk }: { plan: Plan; onAsk: (text: string) => void }) {
  const narrow = useNarrow()
  const { nodes, bands, height } = useMemo(() => layoutPlan(plan.items, narrow ? 3 : 8), [plan.items, narrow])
  const [selected, setSelected] = useState<string | null>(null)
  const lit = usePlanHighlight((s) => s.ids)
  const note = usePlanHighlight((s) => s.note)

  const byId = new Map(nodes.map((n) => [n.item.id, n]))
  const edges = nodes.flatMap((to) =>
    to.item.depends_on.flatMap((fromId) => {
      const from = byId.get(fromId)
      return from ? [{ from, to }] : []
    }),
  )
  const picked = selected ? byId.get(selected) : undefined
  const firstLit = nodes.find((n) => lit.includes(n.item.id))

  // Bring what buddy points at into view.
  useEffect(() => {
    if (!firstLit) return
    document.getElementById(`node-${firstLit.item.id}`)?.scrollIntoView?.({ block: 'center', behavior: 'smooth' })
  }, [firstLit])

  return (
    <>
      <p className="plan-map__meta label">
        Сделано {plan.done} из {plan.total} · нажми на сферу, чтобы открыть пункт
      </p>
      <div className="plan-map__canvas" style={{ height }}>
        {bands.map((b) => (
          <div key={b.period} className="plan-map__band" style={{ top: b.top, height: b.height }}>
            <span className="label">{b.title}</span>
          </div>
        ))}

        <svg className="plan-map__links" viewBox={`0 0 100 ${height}`} preserveAspectRatio="none" aria-hidden="true">
          {edges.map(({ from, to }) => {
            const done = from.item.status === 'PLAN_ITEM_STATUS_DONE'
            const mid = (from.y + to.y) / 2
            return (
              <path
                key={`${from.item.id}>${to.item.id}`}
                data-edge={`${from.item.id}>${to.item.id}`}
                data-done={done}
                className="plan-map__link"
                d={`M ${from.x} ${from.y} C ${from.x} ${mid}, ${to.x} ${mid}, ${to.x} ${to.y}`}
                vectorEffect="non-scaling-stroke"
              />
            )
          })}
        </svg>

        {nodes.map((n, i) => {
          const status = itemStatus[n.item.status]
          const isLit = lit.includes(n.item.id)
          return (
            <button
              key={n.item.id}
              id={`node-${n.item.id}`}
              type="button"
              className="plan-node"
              data-tone={status.tone}
              data-highlight={isLit}
              aria-pressed={selected === n.item.id}
              aria-label={`${n.item.title} · ${status.text}`}
              style={{ left: `${n.x}%`, top: n.y, '--i': i } as CSSProperties}
              onClick={() => setSelected(selected === n.item.id ? null : n.item.id)}
            >
              <span className="plan-node__orb" aria-hidden="true" />
              <span className="plan-node__title" aria-hidden="true">
                {n.item.title}
              </span>
              {isLit && note && n === firstLit && <span className="plan-node__sticker">{note}</span>}
            </button>
          )
        })}
      </div>

      {picked && (
        <PlanItemCard
          key={picked.item.id}
          item={picked.item}
          blockedBy={picked.item.blocked_by.map((id) => byId.get(id)?.item.title ?? id)}
          onAsk={onAsk}
          className="plan-map__card"
        >
          <div className="plan-map__card-head">
            <p className="plan-map__card-title">{picked.item.title}</p>
            <button
              type="button"
              className="plan-map__close"
              aria-label="Закрыть карточку"
              onClick={() => setSelected(null)}
            >
              ×
            </button>
          </div>
        </PlanItemCard>
      )}
    </>
  )
}
