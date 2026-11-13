import { plural, pluralGenitive } from '../../shared/plural'
import { QueryState } from '../../shared/ui/QueryState'
import { dayLabel, itemStatus, periodTitle, useToday } from './useToday'

const blockerKind: Record<string, string> = {
  request: 'Заявка',
  access_missing: 'Нет доступа',
  plan_item: 'Пункт плана',
}

/** Live content of the «Сегодня» orb: stage, next steps and blockers. */
export function TodayPanel({ onAsk }: { onAsk: (text: string) => void }) {
  const today = useToday()

  return (
    <QueryState
      query={today}
      isEmpty={(t) => t.next_steps.length === 0 && t.blockers.length === 0}
      empty="На сейчас всё сделано. Загляни в план или спроси buddy, что дальше."
    >
      {(t) => (
        <>
          <p className="info__meta">
            {[
              dayLabel(t.day, t.week),
              periodTitle[t.period] && `этап «${periodTitle[t.period]}»`,
              `${t.done} из ${t.total} ${pluralGenitive(t.total, ['шага', 'шагов'])}`,
            ]
              .filter(Boolean)
              .join(' · ')}
          </p>

          {t.next_steps.length > 0 && (
            <ul className="info__list" aria-label="Следующие шаги">
              {t.next_steps.map((step) => {
                const status = itemStatus[step.status]
                return (
                  <li key={step.id}>
                    <button
                      type="button"
                      className="info__item info__item--button"
                      onClick={() => onAsk(`Помоги со шагом «${step.title}»: с чего начать?`)}
                    >
                      <span className="info__text">
                        <span className="info__item-title">{step.title}</span>
                        {step.practice && <span className="info__note">{step.practice}</span>}
                      </span>
                      <span className={`info__status info__status--${status.tone}`}>{status.text}</span>
                    </button>
                  </li>
                )
              })}
            </ul>
          )}

          {t.blockers.length > 0 && (
            <>
              <p className="label info__subhead">
                Что мешает · {t.blockers.length} {plural(t.blockers.length, ['пункт', 'пункта', 'пунктов'])}
              </p>
              <ul className="info__list" aria-label="Что мешает">
                {t.blockers.map((b) => (
                  <li key={`${b.kind}:${b.id}`} className="info__item">
                    <span className="info__text">
                      <span className="info__item-title">{b.title}</span>
                      <span className="info__note">{b.reason}</span>
                    </span>
                    <span className="info__status info__status--waiting">{blockerKind[b.kind] ?? 'Блокер'}</span>
                  </li>
                ))}
              </ul>
            </>
          )}
        </>
      )}
    </QueryState>
  )
}
